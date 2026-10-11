package clouddrive2

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"hash"
	"io"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"

	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

type UploadRequest struct {
	File                             *os.File
	Destination, DeviceID, SessionID string
	ExpectedSHA1                     string
	Fence                            func(context.Context) (func(), error)
	OnChunk                          func(int64, int64) error
	OnStart                          func(string) error
	OnProgress                       func(int64, string) error
}

func (a *Adapter) EnsureFolder(ctx context.Context, directory, allowedRoot string) error {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return acquireErr
	}
	defer releaseOperation()
	root := path.Clean(allowedRoot)
	directory = path.Clean(directory)
	if directory != root && !strings.HasPrefix(directory, root+"/") {
		return errors.New("upload directory outside configured root")
	}
	current := root
	file, err := a.Find(ctx, current)
	if err != nil {
		return err
	}
	if !file.IsDirectory {
		return storage.ErrDirectory
	}
	for _, part := range strings.Split(strings.TrimPrefix(directory, root), "/") {
		if part == "" {
			continue
		}
		next := path.Join(current, part)
		existing, err := a.Find(ctx, next)
		if err == nil {
			if !existing.IsDirectory {
				return errors.New("upload folder conflicts with existing file")
			}
			current = next
			continue
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
		_, err = authorized(bounded, a, func(c context.Context) (*pb.CreateFolderResult, error) {
			return a.client.CreateFolder(c, &pb.CreateFolderRequest{ParentPath: current, FolderName: part})
		})
		cancel()
		if err != nil {
			return sanitizeError(err)
		}
		if _, err = a.Find(ctx, next); err != nil {
			return err
		}
		current = next
	}
	return nil
}
func (a *Adapter) UploadFile(ctx context.Context, in UploadRequest) (storage.File, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return storage.File{}, acquireErr
	}
	defer releaseOperation()
	if in.File == nil || in.DeviceID == "" || in.Destination == "" {
		return storage.File{}, errors.New("invalid upload request")
	}
	stat, err := in.File.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return storage.File{}, errors.New("upload source unavailable")
	}
	size := stat.Size()
	if size < 0 {
		return storage.File{}, errors.New("invalid upload size")
	}
	session := in.SessionID
	if session == "" {
		release := func() {}
		if in.Fence != nil {
			release, err = in.Fence(ctx)
			if err != nil {
				return storage.File{}, err
			}
		}
		// Start acknowledgement and OnStart must remain in one exclusive operation.
		released := false
		finish := func() {
			if !released {
				release()
				released = true
			}
		}
		defer finish()
		known := map[uint32]string{}
		if in.ExpectedSHA1 != "" {
			known[uint32(pb.CloudDriveFile_Sha1)] = in.ExpectedSHA1
		}
		bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
		result, err := authorized(bounded, a, func(c context.Context) (*pb.RemoteUploadStarted, error) {
			return a.client.StartRemoteUpload(c, &pb.StartRemoteUploadRequest{FilePath: in.Destination, FileSize: uint64(size), KnownHashes: known, ClientCanCalculateHashes: true})
		})
		cancel()
		if err != nil {
			return storage.File{}, sanitizeError(err)
		}
		if result == nil || result.GetUploadId() == "" {
			return storage.File{}, storage.ErrInvalidResponse
		}
		session = result.GetUploadId()
		if in.OnStart != nil {
			if err = in.OnStart(session); err != nil {
				return storage.File{}, errors.New("upload session persistence failed; do not restart blindly")
			}
		}
		finish()
	}
	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-streamCtx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastActivity.Load())) > 2*time.Minute {
					stop()
					return
				}
			}
		}
	}()
	var processed int64
	lastProgress := time.Time{}
	progress := func(bytes int64, state string) error {
		lastActivity.Store(time.Now().UnixNano())
		processed = max(processed, bytes)
		if in.OnProgress != nil && (time.Since(lastProgress) > time.Second || state != "transfer") {
			lastProgress = time.Now()
			return in.OnProgress(min(processed, size), state)
		}
		return nil
	}
	for {
		if streamCtx.Err() != nil {
			return storage.File{}, storage.ErrUnavailable
		}
		token, err := a.tokens.get(streamCtx)
		if err != nil {
			return storage.File{}, sanitizeError(err)
		}
		stream, err := a.client.RemoteUploadChannel(withToken(streamCtx, token), &pb.RemoteUploadChannelRequest{DeviceId: in.DeviceID})
		if err != nil {
			return storage.File{}, sanitizeError(err)
		}
		for {
			reply, recvErr := stream.Recv()
			if recvErr != nil {
				if status.Code(recvErr) == codes.Unauthenticated && !a.tokens.invalidate(token) {
					return storage.File{}, storage.ErrUnauthenticated
				}
				if status.Code(recvErr) == codes.Unimplemented {
					return storage.File{}, storage.ErrUnsupported
				}
				if streamCtx.Err() != nil {
					return storage.File{}, storage.ErrUnavailable
				}
				break
			}
			lastActivity.Store(time.Now().UnixNano())
			if reply.GetUploadId() != session {
				continue
			}
			switch request := reply.GetRequest().(type) {
			case *pb.RemoteUploadChannelReply_ReadData:
				offset, length := request.ReadData.GetOffset(), request.ReadData.GetLength()
				if offset > uint64(size) || length > uint64(size)-offset {
					return storage.File{}, errors.New("remote read outside upload source or memory limits")
				}
				end := offset + length
				for offset < end {
					if streamCtx.Err() != nil {
						return storage.File{}, storage.ErrUnavailable
					}
					n := min(end-offset, uint64(1<<20))
					data := make([]byte, int(n))
					read, readErr := in.File.ReadAt(data, int64(offset))
					if readErr != nil && readErr != io.EOF {
						return storage.File{}, errors.New("upload source read failed")
					}
					if read != len(data) {
						return storage.File{}, errors.New("upload source changed during transfer")
					}
					chunkEnd := offset + uint64(read)
					bounded, cancel := context.WithTimeout(streamCtx, a.options.RequestTimeout)
					release := func() {}
					if in.Fence != nil {
						release, err = in.Fence(bounded)
						if err != nil {
							cancel()
							return storage.File{}, err
						}
					}
					ack, sendErr := authorized(bounded, a, func(c context.Context) (*pb.RemoteReadDataReply, error) {
						return a.client.RemoteReadData(c, &pb.RemoteReadDataUpload{UploadId: session, Offset: offset, Length: uint64(read), LazyRead: request.ReadData.GetLazyRead(), Data: data, IsLastChunk: chunkEnd == uint64(size)})
					})
					if sendErr == nil && ack != nil && ack.GetSuccess() && ack.GetBytesReceived() == uint64(read) && in.OnChunk != nil {
						sendErr = in.OnChunk(int64(offset), int64(chunkEnd))
					}
					release()
					cancel()
					if sendErr != nil {
						return storage.File{}, sanitizeError(sendErr)
					}
					if ack == nil || !ack.GetSuccess() || ack.GetBytesReceived() != uint64(read) {
						return storage.File{}, storage.ErrInvalidResponse
					}
					offset = chunkEnd
					if err = progress(int64(offset), "transfer"); err != nil {
						return storage.File{}, err
					}
				}
			case *pb.RemoteUploadChannelReply_HashData:
				if err = a.hashUpload(streamCtx, in.File, session, request.HashData, func(n int64) error { return progress(n, "hashing") }, in.Fence); err != nil {
					return storage.File{}, err
				}
			case *pb.RemoteUploadChannelReply_StatusChanged:
				switch request.StatusChanged.GetStatus() {
				case pb.UploadFileInfo_Finish, pb.UploadFileInfo_Skipped:
					current, err := in.File.Stat()
					if err != nil || current.Size() != size || !current.ModTime().Equal(stat.ModTime()) {
						return storage.File{}, errors.New("upload source changed")
					}
					for attempt := 0; attempt < 6; attempt++ {
						remote, err := a.Find(streamCtx, in.Destination)
						if err == nil && !remote.IsDirectory && remote.Size == size {
							if err = progress(size, "verified"); err != nil {
								return storage.File{}, err
							}
							return remote, nil
						}
						select {
						case <-streamCtx.Done():
							return storage.File{}, storage.ErrUnavailable
						case <-time.After(time.Second):
						}
					}
					return storage.File{}, errors.New("remote upload reported completion but file verification failed")
				case pb.UploadFileInfo_Error, pb.UploadFileInfo_FatalError, pb.UploadFileInfo_Cancelled, pb.UploadFileInfo_Ignored:
					return storage.File{}, errors.New("CloudDrive2 upload failed or was cancelled; inspect the stored session")
				case pb.UploadFileInfo_Pause:
					return storage.File{}, errors.New("CloudDrive2 upload paused; resume the stored session")
				default:
					if err = progress(processed, "transfer"); err != nil {
						return storage.File{}, err
					}
				}
			}
		}
		select {
		case <-streamCtx.Done():
			return storage.File{}, storage.ErrUnavailable
		case <-time.After(time.Second):
		}
	}
}
func (a *Adapter) ResumeUpload(ctx context.Context, id string) error {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return acquireErr
	}
	defer releaseOperation()
	bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	_, err := authorized(bounded, a, func(c context.Context) (any, error) {
		return a.client.RemoteUploadControl(c, &pb.RemoteUploadControlRequest{UploadId: id, Control: &pb.RemoteUploadControlRequest_Resume{Resume: &pb.ResumeRemoteUpload{}}})
	})
	return sanitizeUploadError(err)
}
func (a *Adapter) hashUpload(ctx context.Context, file *os.File, id string, request *pb.RemoteHashDataRequest, progress func(int64) error, fences ...func(context.Context) (func(), error)) error {
	stat, err := file.Stat()
	if err != nil {
		return errors.New("upload source unavailable")
	}
	size := stat.Size()
	kind := pb.CloudDriveFile_HashType(request.GetHashType())
	var whole hash.Hash
	switch kind {
	case pb.CloudDriveFile_Md5:
		whole = md5.New()
	case pb.CloudDriveFile_Sha1, pb.CloudDriveFile_PikPakSha1:
		whole = sha1.New()
	default:
		return errors.New("unsupported remote hash algorithm")
	}
	blockSize := int64(0)
	if kind == pb.CloudDriveFile_Md5 {
		blockSize = int64(request.GetBlockSize())
		if blockSize > 64<<20 || (blockSize > 0 && size/blockSize > 100000) {
			return errors.New("remote MD5 block request exceeds limits")
		}
	}
	segment := int64(1 << 20)
	if blockSize > 0 {
		segment = blockSize
	}
	if kind == pb.CloudDriveFile_PikPakSha1 {
		segment = 256 << 10
		if size > 128<<20 {
			segment = 512 << 10
		}
		if size > 256<<20 {
			segment = 1 << 20
		}
		if size > 512<<20 {
			segment = 2 << 20
		}
	}
	blocks := []string{}
	var offset int64
	lastReport := time.Time{}
	report := func(final bool, hashValue string) error {
		bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
		defer cancel()
		message := &pb.RemoteHashProgressUpload{UploadId: id, BytesHashed: uint64(offset), TotalBytes: uint64(size), HashType: kind}
		if final {
			message.HashValue = &hashValue
			message.BlockHashes = blocks
		}
		release := func() {}
		if len(fences) > 0 && fences[0] != nil {
			var err error
			release, err = fences[0](bounded)
			if err != nil {
				return err
			}
		}
		defer release()
		_, err := authorized(bounded, a, func(c context.Context) (any, error) { return a.client.RemoteHashProgress(c, message) })
		return sanitizeUploadError(err)
	}
	for offset < size {
		if ctx.Err() != nil {
			return storage.ErrUnavailable
		}
		n := min(segment, size-offset)
		data := make([]byte, int(n))
		count, readErr := file.ReadAt(data, offset)
		if readErr != nil && readErr != io.EOF || int64(count) != n {
			return errors.New("upload hash read failed")
		}
		if kind == pb.CloudDriveFile_PikPakSha1 {
			part := sha1.Sum(data)
			whole.Write(part[:])
		} else {
			whole.Write(data)
		}
		if blockSize > 0 {
			part := md5.Sum(data)
			blocks = append(blocks, hex.EncodeToString(part[:]))
		}
		offset += n
		if err = progress(offset); err != nil {
			return err
		}
		if time.Since(lastReport) > time.Second {
			if err = report(false, ""); err != nil {
				return err
			}
			lastReport = time.Now()
		}
	}
	value := hex.EncodeToString(whole.Sum(nil))
	if kind == pb.CloudDriveFile_PikPakSha1 {
		value = strings.ToUpper(value)
	}
	return report(true, value)
}
func sanitizeUploadError(err error) error {
	if err == nil {
		return nil
	}
	return sanitizeError(err)
}
