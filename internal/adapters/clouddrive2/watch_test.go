package clouddrive2

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

type pushFixture struct {
	pb.UnimplementedCloudDriveFileSrvServer
	calls    atomic.Int32
	messages []*pb.CloudDrivePushMessage
	fail     codes.Code
	idle     bool
}

func (s *pushFixture) PushMessage(_ *emptypb.Empty, stream grpc.ServerStreamingServer[pb.CloudDrivePushMessage]) error {
	s.calls.Add(1)
	md, _ := metadata.FromIncomingContext(stream.Context())
	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer owned-test" {
		return status.Error(codes.Unauthenticated, "PRIVATE-TOKEN")
	}
	if s.fail != codes.OK {
		return status.Error(s.fail, "PRIVATE-URL?signature=private")
	}
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return err
	}
	for _, m := range s.messages {
		if err := stream.Send(m); err != nil {
			return err
		}
	}
	if s.idle {
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	return nil
}
func pushChange(kind pb.FileSystemChange_ChangeType, old, new string) *pb.CloudDrivePushMessage {
	return &pb.CloudDrivePushMessage{MessageType: pb.CloudDrivePushMessage_FILE_SYSTEM_CHANGE, Data: &pb.CloudDrivePushMessage_FileSystemChange{FileSystemChange: &pb.FileSystemChange{ChangeType: kind, Path: old, NewPath: &new}}}
}
func pushAdapter(t *testing.T, s *pushFixture) *Adapter {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, s)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "owned-test", RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
func TestPushMessageSanitizedHintsAndCallbackFailure(t *testing.T) {
	s := &pushFixture{messages: []*pb.CloudDrivePushMessage{
		pushChange(pb.FileSystemChange_CREATE, "/115/媒体/own.mkv", ""), pushChange(pb.FileSystemChange_RENAME, "/115/媒体/old.mkv", "/115/new.mkv"),
		pushChange(pb.FileSystemChange_DELETE, "/115/媒体/gone.mkv", ""), pushChange(pb.FileSystemChange_MODIFY, "/115/媒体/own.mkv", ""),
		pushChange(pb.FileSystemChange_CREATE, "/115/../escape.mkv", ""), pushChange(pb.FileSystemChange_RENAME, "/115/old", ""),
		{MessageType: pb.CloudDrivePushMessage_LOG_MESSAGE, Data: &pb.CloudDrivePushMessage_LogMessage{LogMessage: &pb.LogMessage{}}},
	}}
	a := pushAdapter(t, s)
	var ready int
	var hints []FileChange
	if err := a.ObserveChanges(context.Background(), func() error { ready++; return nil }, func(h FileChange) error { hints = append(hints, h); return nil }); err != nil {
		t.Fatal(err)
	}
	if ready != 1 || len(hints) != 4 || hints[1].NewPath != "/115/new.mkv" {
		t.Fatal("invalid or missing hints", ready, hints)
	}
	ownErr := errors.New("own callback stopped")
	if err := a.WatchChanges(context.Background(), func(FileChange) error { return ownErr }); !errors.Is(err, ownErr) {
		t.Fatal("callback error lost", err)
	}
	if err := a.WatchChanges(context.Background(), nil); !errors.Is(err, storage.ErrInvalidResponse) {
		t.Fatal("nil visitor accepted")
	}
}
func TestPushUnavailableAndCancellationAreBoundedAndSecretFree(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want error
	}{{codes.Unimplemented, storage.ErrUnsupported}, {codes.PermissionDenied, storage.ErrForbidden}, {codes.Unauthenticated, storage.ErrUnauthenticated}, {codes.Unavailable, storage.ErrUnavailable}} {
		t.Run(tc.code.String(), func(t *testing.T) {
			s := &pushFixture{fail: tc.code}
			a := pushAdapter(t, s)
			err := a.WatchChanges(context.Background(), func(FileChange) error { return nil })
			if !errors.Is(err, tc.want) || err.Error() == "PRIVATE-URL?signature=private" {
				t.Fatal(err)
			}
			if s.calls.Load() != 1 {
				t.Fatal("static token replayed")
			}
		})
	}
	s := &pushFixture{idle: true, messages: []*pb.CloudDrivePushMessage{{MessageType: pb.CloudDrivePushMessage_DOWNLOADER_COUNT}}}
	a := pushAdapter(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- a.ObserveChanges(ctx, func() error { close(ready); return nil }, func(FileChange) error { return nil })
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream not connected")
	}
	a.Retire()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream outlived cancellation")
	}
}
func TestUnsafePushPathsNeverBecomeHints(t *testing.T) {
	for _, p := range []string{"", "relative", "/115//x", "/115/a/../x", "/115/x\\name", "/115/x\nname", "/115/x\x00"} {
		if _, ok := mapChange(pushChange(pb.FileSystemChange_CREATE, p, "")); ok {
			t.Fatal("unsafe path", p)
		}
	}
	unknown := pushChange(pb.FileSystemChange_ChangeType(99), "/115/x", "")
	if _, ok := mapChange(unknown); ok {
		t.Fatal("unknown event treated as create")
	}
}

func TestMountAndCloudAccountChangeHintsNeverContainAccountData(t *testing.T) {
	newPath := "/115/NewMount"
	m := &pb.CloudDrivePushMessage{MessageType: pb.CloudDrivePushMessage_CLOUD_API_CHANGE, Data: &pb.CloudDrivePushMessage_CloudApiChange{CloudApiChange: &pb.CloudApiChange{Action: pb.CloudApiChange_RENAME, CloudName: "SECRET-ACCOUNT", UserName: "PRIVATE-USER", MountPath: "/115/OldMount", NewMountPath: &newPath}}}
	hint, ok := mapChange(m)
	if !ok || !hint.Directory || hint.Path != "/115/OldMount" || hint.NewPath != newPath {
		t.Fatal("rename mount hint absent")
	}
	for _, success := range []bool{false, true} {
		m = &pb.CloudDrivePushMessage{MessageType: pb.CloudDrivePushMessage_MOUNT_POINT_CHANGE, Data: &pb.CloudDrivePushMessage_MountPointChange{MountPointChange: &pb.MountPointChange{Success: success, MountPoint: "/115/Media", FailReason: "SECRET-FAILURE"}}}
		hint, ok = mapChange(m)
		if ok != success {
			t.Fatal("failed mount treated as authoritative")
		}
	}
}
