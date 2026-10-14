package clouddrive2

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"net"
	"testing"
	"time"
)

type accountFixture struct {
	pb.UnimplementedCloudDriveFileSrvServer
}

func (*accountFixture) GetSpaceInfo(_ context.Context, r *pb.FileRequest) (*pb.SpaceInfo, error) {
	if r.Path == "/115/missing" {
		return nil, status.Error(codes.Unimplemented, "unsupported")
	}
	return &pb.SpaceInfo{TotalSpace: 1000, UsedSpace: 200, FreeSpace: 800}, nil
}
func (*accountFixture) GetCloudMemberships(_ context.Context, r *pb.FileRequest) (*pb.CloudMemberships, error) {
	if r.Path == "/115/missing" {
		return nil, status.Error(codes.Unimplemented, "unsupported")
	}
	return &pb.CloudMemberships{Memberships: []*pb.CloudMembership{{Identity: "115VIP", ExpireTime: timestamppb.New(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))}}}, nil
}
func TestAccountInfoSeparatesUnknownSpaceFromRealZero(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(srv, &accountFixture{})
	go srv.Serve(listener)
	defer srv.Stop()
	defer listener.Close()
	a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	out, err := a.AccountInfo(context.Background(), "/115/Media")
	if err != nil || out.Space == nil || out.Space.Free != 800 || len(out.Memberships) != 1 || out.Memberships[0].ExpiresAt == nil {
		t.Fatal(out, err)
	}
	out, err = a.AccountInfo(context.Background(), "/115/missing")
	if err != nil || out.Space != nil || out.SpaceError == "" || out.MembershipError == "" {
		t.Fatal(out, err)
	}
}
