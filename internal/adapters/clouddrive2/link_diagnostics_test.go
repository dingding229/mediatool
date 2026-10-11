package clouddrive2

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"net"
	"strings"
	"testing"
)

type diagnosticFixture struct {
	pb.UnimplementedCloudDriveFileSrvServer
}

func (diagnosticFixture) GetRuntimeInfo(context.Context, *emptypb.Empty) (*pb.RuntimeInfo, error) {
	return &pb.RuntimeInfo{ProductVersion: "fixture-1.1.2"}, nil
}
func (diagnosticFixture) FindFileByPath(context.Context, *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	return &pb.CloudDriveFile{IsCloudFile: true, CloudAPI: &pb.CloudAPI{Name: "115open", UserName: "PRIVATE-ACCOUNT"}}, nil
}
func (diagnosticFixture) GetCloudAPIConfig(context.Context, *pb.GetCloudAPIConfigRequest) (*pb.CloudAPIConfig, error) {
	return &pb.CloudAPIConfig{SupportDirectLink: proto.Bool(false), SupportDirectDownloadUrl: proto.Bool(true), DataProxy: &pb.ProxyInfo{Username: proto.String("PRIVATE-PROXY-USER"), Password: proto.String("PRIVATE-PROXY-PASSWORD")}}, nil
}
func (diagnosticFixture) GetDownloadUrlPath(context.Context, *pb.GetDownloadUrlPathRequest) (*pb.DownloadUrlPathInfo, error) {
	return &pb.DownloadUrlPathInfo{DownloadUrlPath: "/static/private?secret=PRIVATE-RELAY", DirectUrl: proto.String("https://cdn.example/file?secret=PRIVATE-DIRECT"), AdditionalHeaders: map[string]string{"Cookie": "PRIVATE-COOKIE"}}, nil
}
func TestLinkDiagnosticsOnlyExposeCapabilityFields(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, diagnosticFixture{})
	go server.Serve(listener)
	defer server.Stop()
	a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "PRIVATE-API-TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	result, err := a.DiagnoseLink(context.Background(), "/115/private.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if !result.DirectURLPresent || !result.RelayPathPresent || !result.CloudConfigReadable || result.DirectLinkEnabled || !result.ProviderSupportsDirectURL {
		t.Fatal("incorrect capabilities", result)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "https://") {
		t.Fatal("diagnostic exposed credentials or signed URL")
	}
}

type httpsPreferenceFixture struct {
	diagnosticFixture
	deny  bool
	calls int
}

func (s *httpsPreferenceFixture) GetCloudAPIConfig(context.Context, *pb.GetCloudAPIConfigRequest) (*pb.CloudAPIConfig, error) {
	return &pb.CloudAPIConfig{UseHttpDownload: proto.Bool(true), SupportDirectLink: proto.Bool(true), MaxDownloadThreads: 2, ForceIpv4: true}, nil
}
func (s *httpsPreferenceFixture) SetCloudAPIConfig(_ context.Context, in *pb.SetCloudAPIConfigRequest) (*emptypb.Empty, error) {
	s.calls++
	if s.deny {
		return nil, status.Error(codes.PermissionDenied, "PRIVATE-provider-detail")
	}
	if in.Config.GetUseHttpDownload() || !in.Config.GetSupportDirectLink() || in.Config.GetMaxDownloadThreads() != 2 || !in.Config.GetForceIpv4() {
		return nil, status.Error(codes.InvalidArgument, "unexpected setting change")
	}
	return &emptypb.Empty{}, nil
}
func TestRequireHTTPSOnlyChangesPlaintextPreferenceAndHonorsPermissions(t *testing.T) {
	for _, deny := range []bool{false, true} {
		fixture := &httpsPreferenceFixture{deny: deny}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := grpc.NewServer()
		pb.RegisterCloudDriveFileSrvServer(server, fixture)
		go server.Serve(listener)
		a, err := New(Options{Endpoint: "http://" + listener.Addr().String(), Token: "PRIVATE-TOKEN"})
		if err != nil {
			t.Fatal(err)
		}
		changed, err := a.RequireHTTPSDownloads(context.Background(), "/115/file")
		a.Close()
		server.Stop()
		if deny {
			if changed || !errors.Is(err, storage.ErrForbidden) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("permission denial bypassed or leaked")
			}
		} else if !changed || err != nil || fixture.calls != 1 {
			t.Fatal("HTTPS update failed", err)
		}
	}
}
