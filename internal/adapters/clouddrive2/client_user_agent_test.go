package clouddrive2

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

type uaFixture struct {
	diagnosticFixture
	config *pb.CloudAPIConfig
	deny   bool
	calls  int
}

func (s *uaFixture) GetCloudAPIConfig(context.Context, *pb.GetCloudAPIConfigRequest) (*pb.CloudAPIConfig, error) {
	return proto.Clone(s.config).(*pb.CloudAPIConfig), nil
}
func (s *uaFixture) SetCloudAPIConfig(_ context.Context, r *pb.SetCloudAPIConfigRequest) (*emptypb.Empty, error) {
	s.calls++
	if s.deny {
		return nil, status.Error(codes.PermissionDenied, "PRIVATE")
	}
	s.config = proto.Clone(r.Config).(*pb.CloudAPIConfig)
	return &emptypb.Empty{}, nil
}
func TestNativeUserAgentOnlyChangesPreferenceAndRestoresOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		deny, fail bool
	}{{"accepted", false, false}, {"not-effective", false, true}, {"permission-denied", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &uaFixture{deny: tc.deny, config: &pb.CloudAPIConfig{SupportDirectLink: proto.Bool(true), UseHttpDownload: proto.Bool(false), MaxDownloadThreads: 2}}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			pb.RegisterCloudDriveFileSrvServer(server, fixture)
			go server.Serve(listener)
			defer server.Stop()
			a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "fixture-token"})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verified := false
			changed, err := a.TryDownloadUserAgent(ctx, "/115/file", "SenPlayer/6.2.1", func(context.Context) error {
				verified = true
				if tc.fail {
					// Simulate another current setting change through the same public API.
					cfg, e := authorized(ctx, a, func(c context.Context) (*pb.CloudAPIConfig, error) {
						return a.client.GetCloudAPIConfig(c, &pb.GetCloudAPIConfigRequest{})
					})
					if e != nil {
						return e
					}
					cfg.MaxDownloadThreads = 7
					_, e = authorized(ctx, a, func(c context.Context) (*emptypb.Empty, error) {
						return a.client.SetCloudAPIConfig(c, &pb.SetCloudAPIConfigRequest{Config: cfg})
					})
					if e != nil {
						return e
					}
					cancel()
					return errors.New("UA download verification failed")
				}
				return nil
			})
			if tc.deny {
				if changed || verified || !errors.Is(err, storage.ErrForbidden) {
					t.Fatal("permission denied bypassed", changed, err)
				}
			} else if tc.fail {
				if changed || err == nil || fixture.config.CustomUserAgent != nil || fixture.config.MaxDownloadThreads != 7 {
					t.Fatal("preference not restored independently", changed, err)
				}
			} else if !changed || err != nil || fixture.config.GetCustomUserAgent() != "SenPlayer/6.2.1" || fixture.config.MaxDownloadThreads != 2 {
				t.Fatal("UA preference not applied", changed, err)
			}
			if !fixture.config.GetSupportDirectLink() || fixture.config.GetUseHttpDownload() {
				t.Fatal("unrelated preference changed")
			}
		})
	}
}
