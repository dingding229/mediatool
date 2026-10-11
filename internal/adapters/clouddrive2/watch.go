package clouddrive2

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

// FileChange is only a hint to reconcile a directory, never an index mutation.
// Paths are validated; provider messages, tokens and file bodies are not exposed.
type FileChange struct {
	Path, NewPath string
	Directory     bool
}

func canonicalChangePath(raw string) bool {
	return raw != "" && len(raw) <= 4096 && strings.HasPrefix(raw, "/") && path.Clean(raw) == raw && !strings.ContainsAny(raw, "\\\x00\r\n")
}

func mapChange(message *pb.CloudDrivePushMessage) (FileChange, bool) {
	if message == nil {
		return FileChange{}, false
	}
	if message.GetMessageType() == pb.CloudDrivePushMessage_MOUNT_POINT_CHANGE {
		mount := message.GetMountPointChange()
		if mount != nil && mount.GetSuccess() && canonicalChangePath(mount.GetMountPoint()) {
			return FileChange{Path: mount.GetMountPoint(), Directory: true}, true
		}
		return FileChange{}, false
	}
	if message.GetMessageType() == pb.CloudDrivePushMessage_CLOUD_API_CHANGE {
		cloud := message.GetCloudApiChange()
		if cloud == nil || !canonicalChangePath(cloud.GetMountPath()) {
			return FileChange{}, false
		}
		hint := FileChange{Path: cloud.GetMountPath(), Directory: true}
		switch cloud.GetAction() {
		case pb.CloudApiChange_ADD, pb.CloudApiChange_REMOVE:
		case pb.CloudApiChange_RENAME:
			if !canonicalChangePath(cloud.GetNewMountPath()) {
				return FileChange{}, false
			}
			hint.NewPath = cloud.GetNewMountPath()
		default:
			return FileChange{}, false
		}
		return hint, true
	}
	if message.GetMessageType() != pb.CloudDrivePushMessage_FILE_SYSTEM_CHANGE {
		return FileChange{}, false
	}
	change := message.GetFileSystemChange()
	if change == nil || !canonicalChangePath(change.GetPath()) {
		return FileChange{}, false
	}
	out := FileChange{Path: change.GetPath(), Directory: change.GetIsDirectory()}
	switch change.GetChangeType() {
	case pb.FileSystemChange_CREATE, pb.FileSystemChange_DELETE, pb.FileSystemChange_MODIFY:
	case pb.FileSystemChange_RENAME:
		if !canonicalChangePath(change.GetNewPath()) {
			return FileChange{}, false
		}
		out.NewPath = change.GetNewPath()
	default:
		return FileChange{}, false
	}
	return out, true
}

// WatchChanges uses the pinned official PushMessage RPC. Each admission is at
// most one minute so retired connections/credentials cannot remain attached to
// an infinite stream. Reconnect/periodic reconciliation is owned by the caller.
func (a *Adapter) WatchChanges(ctx context.Context, visit func(FileChange) error) error {
	return a.watchChanges(ctx, nil, visit)
}

// ObserveChanges confirms the first successful response frame before notifying
// the coordinator; headers alone can precede a trailers-only RPC rejection.
func (a *Adapter) ObserveChanges(ctx context.Context, ready func() error, visit func(FileChange) error) error {
	return a.watchChanges(ctx, ready, visit)
}
func (a *Adapter) watchChanges(ctx context.Context, ready func() error, visit func(FileChange) error) error {
	if visit == nil {
		return storage.ErrInvalidResponse
	}
	ctx, release, err := a.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	bounded, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		token, err := a.tokens.get(bounded)
		if err != nil {
			return sanitizeError(err)
		}
		stream, err := a.client.PushMessage(withToken(bounded, token), &emptypb.Empty{})
		if err == nil {
			_, err = stream.Header()
		}
		if err == nil {
			readySent := false
			for {
				message, recvErr := stream.Recv()
				if recvErr != nil {
					err = recvErr
					break
				}
				if !readySent {
					if ready != nil {
						if readyErr := ready(); readyErr != nil {
							return readyErr
						}
					}
					readySent = true
				}
				if hint, ok := mapChange(message); ok {
					if callbackErr := visit(hint); callbackErr != nil {
						return callbackErr
					}
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		// Account-token refresh only, once. A static API Token is never replaced.
		if attempt == 0 && status.Code(err) == codes.Unauthenticated && a.tokens.invalidate(token) {
			continue
		}
		if bounded.Err() != nil {
			return bounded.Err()
		}
		return sanitizeError(err)
	}
	return storage.ErrUnauthenticated
}
