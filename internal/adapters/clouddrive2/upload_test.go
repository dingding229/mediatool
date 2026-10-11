package clouddrive2

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type uploadFixture struct {
	pb.UnimplementedCloudDriveFileSrvServer
	mu       sync.Mutex
	data     []byte
	dest     string
	result   []byte
	complete chan struct{}
	hashDone chan string
}

func (s *uploadFixture) StartRemoteUpload(ctx context.Context, in *pb.StartRemoteUploadRequest) (*pb.RemoteUploadStarted, error) {
	s.mu.Lock()
	s.dest = in.FilePath
	s.mu.Unlock()
	if in.FileSize != uint64(len(s.data)) || !in.ClientCanCalculateHashes {
		return nil, status.Error(codes.InvalidArgument, "bad size")
	}
	return &pb.RemoteUploadStarted{UploadId: "upload-1"}, nil
}
func (s *uploadFixture) RemoteUploadChannel(in *pb.RemoteUploadChannelRequest, stream grpc.ServerStreamingServer[pb.RemoteUploadChannelReply]) error {
	if in.DeviceId != "persistent-device" {
		return status.Error(codes.InvalidArgument, "unstable device")
	}
	stream.Send(&pb.RemoteUploadChannelReply{UploadId: "upload-1", Request: &pb.RemoteUploadChannelReply_HashData{HashData: &pb.RemoteHashDataRequest{HashType: uint32(pb.CloudDriveFile_Sha1)}}})
	select {
	case hash := <-s.hashDone:
		expected := sha1.Sum(s.data)
		if hash != hex.EncodeToString(expected[:]) {
			return status.Error(codes.InvalidArgument, "hash mismatch")
		}
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	stream.Send(&pb.RemoteUploadChannelReply{UploadId: "upload-1", Request: &pb.RemoteUploadChannelReply_ReadData{ReadData: &pb.RemoteReadDataRequest{Length: uint64(len(s.data))}}})
	select {
	case <-s.complete:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	return stream.Send(&pb.RemoteUploadChannelReply{UploadId: "upload-1", Request: &pb.RemoteUploadChannelReply_StatusChanged{StatusChanged: &pb.RemoteUploadStatusChanged{Status: pb.UploadFileInfo_Finish}}})
}
func (s *uploadFixture) RemoteHashProgress(ctx context.Context, in *pb.RemoteHashProgressUpload) (*pb.RemoteHashProgressReply, error) {
	if in.HashValue != nil {
		s.hashDone <- *in.HashValue
	}
	return &pb.RemoteHashProgressReply{}, nil
}
func (s *uploadFixture) RemoteReadData(ctx context.Context, in *pb.RemoteReadDataUpload) (*pb.RemoteReadDataReply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(in.Offset) != len(s.result) {
		return nil, status.Error(codes.InvalidArgument, "offset")
	}
	s.result = append(s.result, in.Data...)
	if len(s.result) == len(s.data) {
		close(s.complete)
	}
	return &pb.RemoteReadDataReply{Success: true, BytesReceived: uint64(len(in.Data))}, nil
}
func (s *uploadFixture) FindFileByPath(ctx context.Context, in *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.result) != len(s.data) {
		return nil, status.Error(codes.NotFound, "not yet uploaded")
	}
	h := sha1.Sum(s.result)
	return &pb.CloudDriveFile{Id: "uploaded", Name: "media.mkv", FullPathName: s.dest, Size: int64(len(s.result)), FileType: pb.CloudDriveFile_File, FileHashes: map[uint32]string{2: hex.EncodeToString(h[:])}}, nil
}
func TestRemoteUploadProtocolDataHashAndVerification(t *testing.T) {
	data := []byte("owned-test-media")
	fixture := &uploadFixture{data: data, complete: make(chan struct{}), hashDone: make(chan string, 1)}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, fixture)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	adapter, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	source := filepath.Join(t.TempDir(), "media.mkv")
	os.WriteFile(source, data, 0600)
	file, _ := os.Open(source)
	defer file.Close()
	started := ""
	result, err := adapter.UploadFile(context.Background(), UploadRequest{File: file, Destination: "/115/Media/media.mkv", DeviceID: "persistent-device", OnStart: func(id string) error { started = id; return nil }})
	if err != nil || started != "upload-1" || result.Size != int64(len(data)) {
		t.Fatal(err, result)
	}
	if string(fixture.result) != string(data) {
		t.Fatal("uploaded wrong bytes")
	}
	if _, err = adapter.Upload(context.Background(), source, "/115"); err == nil || errors.Is(err, storage.ErrNotImplemented) {
		t.Fatal("unsafe legacy upload API accepted")
	}
}
