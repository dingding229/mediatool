package clouddrive2

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "media-tool/internal/adapters/clouddrive2/proto"
)

type mutationServer struct {
	pb.UnimplementedCloudDriveFileSrvServer
	calls  atomic.Int32
	reject bool
	t      *testing.T
}

func (s *mutationServer) check(ctx context.Context) error {
	s.calls.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer OWNED-TOKEN" {
		return status.Error(codes.PermissionDenied, "PRIVATE-DETAIL")
	}
	if s.reject {
		return status.Error(codes.Unauthenticated, "PRIVATE-DETAIL")
	}
	return nil
}
func (s *mutationServer) RenameFile(ctx context.Context, in *pb.RenameFileRequest) (*pb.FileOperationResult, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	if in.GetTheFilePath() != "/115/Owned/source.mkv" || in.GetNewName() != "custom.mkv" {
		return nil, status.Error(codes.InvalidArgument, "invalid fixture")
	}
	return &pb.FileOperationResult{Success: true}, nil
}
func (s *mutationServer) MoveFile(ctx context.Context, in *pb.MoveFileRequest) (*pb.FileOperationResult, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	if in.ConflictPolicy == nil || in.GetConflictPolicy() != pb.MoveFileRequest_Skip || in.MoveAcrossClouds == nil || in.GetMoveAcrossClouds() || len(in.GetTheFilePaths()) != 1 || in.GetDestPath() != "/115/Owned/Movies" {
		return nil, status.Error(codes.InvalidArgument, "unsafe move")
	}
	return &pb.FileOperationResult{Success: true}, nil
}
func (s *mutationServer) CreateFolder(ctx context.Context, in *pb.CreateFolderRequest) (*pb.CreateFolderResult, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	return &pb.CreateFolderResult{Result: &pb.FileOperationResult{Success: true}}, nil
}
func TestCloudFileMutationsUseSkipAndNeverReplayAuthenticationFailure(t *testing.T) {
	s := &mutationServer{t: t}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(service, s)
	go service.Serve(listener)
	defer service.Stop()
	defer listener.Close()
	a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "OWNED-TOKEN", RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.RenamePath(context.Background(), "/115/Owned/source.mkv", "custom.mkv"); err != nil {
		t.Fatal(err)
	}
	if err = a.MovePath(context.Background(), "/115/Owned/custom.mkv", "/115/Owned/Movies"); err != nil {
		t.Fatal(err)
	}
	if err = a.MakeDirectory(context.Background(), "/115/Owned", "Movies"); err != nil {
		t.Fatal(err)
	}
	before := s.calls.Load()
	s.reject = true
	err = a.RenamePath(context.Background(), "/115/Owned/source.mkv", "custom.mkv")
	if err == nil || strings.Contains(err.Error(), "PRIVATE-DETAIL") || s.calls.Load() != before+1 {
		t.Fatal("mutation replayed or provider detail leaked", err)
	}
}
