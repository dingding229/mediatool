package clouddrive2

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

type protocolServer struct {
	pb.UnimplementedCloudDriveFileSrvServer
	tokenCalls                                                        atomic.Int32
	mu                                                                sync.Mutex
	currentToken                                                      string
	fileType                                                          pb.CloudDriveFile_FileType
	withoutURL, notReady, badLogin, expiredLogin, shortLink, noExpiry bool
	code                                                              codes.Code
	delay                                                             time.Duration
}

func (s *protocolServer) auth(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	expected := "Bearer " + s.currentToken
	s.mu.Unlock()
	values := md.Get("authorization")
	if len(values) != 1 || values[0] != expected {
		return status.Error(codes.Unauthenticated, "SECRET should never be returned to API users")
	}
	return nil
}
func (s *protocolServer) GetSystemInfo(context.Context, *emptypb.Empty) (*pb.CloudDriveSystemInfo, error) {
	return &pb.CloudDriveSystemInfo{IsLogin: !s.notReady, SystemReady: !s.notReady}, nil
}
func (s *protocolServer) GetToken(_ context.Context, req *pb.GetTokenRequest) (*pb.JWTToken, error) {
	s.tokenCalls.Add(1)
	if s.badLogin || req.GetUserName() != "user" || req.GetPassword() != "password" {
		return &pb.JWTToken{Success: false, ErrorMessage: "SECRET upstream login error"}, nil
	}
	time.Sleep(10 * time.Millisecond)
	expiration := time.Now().Add(time.Hour)
	if s.expiredLogin {
		expiration = time.Now().Add(-time.Minute)
	}
	s.mu.Lock()
	s.currentToken = "account-token"
	s.mu.Unlock()
	return &pb.JWTToken{Success: true, Token: "account-token", Expiration: timestamppb.New(expiration)}, nil
}
func (s *protocolServer) FindFileByPath(ctx context.Context, req *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	if err := s.auth(ctx); err != nil {
		return nil, err
	}
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "SECRET")
		case <-time.After(s.delay):
		}
	}
	if s.code != codes.OK {
		return nil, status.Error(s.code, "SECRET provider error")
	}
	if req.GetPath() == "/missing" {
		return nil, status.Error(codes.NotFound, "SECRET")
	}
	return &pb.CloudDriveFile{Id: "cloud-file-not-pickcode", Name: "media.mkv", FullPathName: req.GetPath(), Size: 1024, FileType: s.fileType}, nil
}
func (s *protocolServer) GetDownloadUrlPath(ctx context.Context, req *pb.GetDownloadUrlPathRequest) (*pb.DownloadUrlPathInfo, error) {
	if err := s.auth(ctx); err != nil {
		return nil, err
	}
	if !req.GetGetDirectUrl() || req.GetPreview() || req.GetLazyRead() {
		return nil, status.Error(codes.InvalidArgument, "invalid flags")
	}
	result := &pb.DownloadUrlPathInfo{DirectUrl: proto.String("https://cdn.example.com/video?temporary=token"), UserAgent: proto.String("required-agent"), AdditionalHeaders: map[string]string{"Referer": "https://115.example.test/"}}
	if !s.noExpiry {
		seconds := uint64(60)
		if s.shortLink {
			seconds = 0
		}
		result.ExpiresIn = &seconds
	}
	if s.withoutURL {
		result.DirectUrl = nil
		result.DownloadUrlPath = "/static/http/internal/false/video?token=ADMIN-SECRET"
	}
	return result, nil
}
func (s *protocolServer) GetSubFiles(req *pb.ListSubFileRequest, stream grpc.ServerStreamingServer[pb.SubFilesReply]) error {
	if err := s.auth(stream.Context()); err != nil {
		return err
	}
	for _, name := range []string{"a.mkv", "b.mkv"} {
		if err := stream.Send(&pb.SubFilesReply{SubFiles: []*pb.CloudDriveFile{{Id: name, Name: name, FullPathName: req.GetPath() + "/" + name, FileType: pb.CloudDriveFile_File, Size: 10}}}); err != nil {
			return err
		}
	}
	return nil
}
func testAdapter(t *testing.T, server *protocolServer, opts Options) *Adapter {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(service, server)
	go service.Serve(listener)
	opts.Endpoint = "http://" + listener.Addr().String()
	adapter, err := New(opts)
	if err != nil {
		service.Stop()
		t.Fatal(err)
	}
	t.Cleanup(func() { adapter.Close(); service.Stop(); listener.Close() })
	return adapter
}
func TestOfficialProtocolStaticAuthenticationAndDownloadFields(t *testing.T) {
	server := &protocolServer{currentToken: "test-token", fileType: pb.CloudDriveFile_File}
	adapter := testAdapter(t, server, Options{Token: "test-token"})
	result, err := adapter.Probe(context.Background(), "/115/Media/media.mkv")
	if err != nil || !result.PathReadable {
		t.Fatal(err, result)
	}
	link, err := adapter.Resolve(context.Background(), storage.FileRef{Path: "/115/Media/中文 & 特别版.mkv", PickCode: "notUsedAsCloudID"})
	if err != nil {
		t.Fatal(err)
	}
	if link.UserAgent != "required-agent" || link.Headers["Referer"] == "" || time.Until(link.ExpiresAt) > time.Minute || link.URL == "" {
		t.Fatal("download response fields not preserved")
	}
	if server.tokenCalls.Load() != 0 {
		t.Fatal("static token caused unexpected account login")
	}
	files, err := adapter.List(context.Background(), "/115/Media")
	if err != nil || len(files) != 2 {
		t.Fatal(err, len(files))
	}
	if _, err = adapter.Upload(context.Background(), "a", "b"); err == nil {
		t.Fatal("upload falsely implemented")
	}
}
func TestAccountTokenSingleflightAndUnauthorizedRefresh(t *testing.T) {
	server := &protocolServer{fileType: pb.CloudDriveFile_File}
	adapter := testAdapter(t, server, Options{Username: "user", Password: "password"})
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := adapter.Find(context.Background(), "/115/Media/a.mkv"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if server.tokenCalls.Load() != 1 {
		t.Fatal("duplicate token requests", server.tokenCalls.Load())
	}
	server.mu.Lock()
	server.currentToken = "revoke-old-token"
	server.mu.Unlock()
	if _, err := adapter.Find(context.Background(), "/115/Media/a.mkv"); err != nil {
		t.Fatal(err)
	}
	if server.tokenCalls.Load() != 2 {
		t.Fatal("authentication failure did not refresh once")
	}
}
func TestProviderErrorsAreSanitizedAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		server *protocolServer
		opts   Options
		target error
	}{
		{"auth", &protocolServer{currentToken: "other", fileType: pb.CloudDriveFile_File}, Options{Token: "bad"}, storage.ErrUnauthenticated},
		{"permission", &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File, code: codes.PermissionDenied}, Options{Token: "token"}, storage.ErrForbidden},
		{"version", &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File, code: codes.Unimplemented}, Options{Token: "token"}, storage.ErrUnsupported},
		{"deadline", &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File, delay: time.Second}, Options{Token: "token", RequestTimeout: 20 * time.Millisecond}, storage.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := testAdapter(t, tc.server, tc.opts)
			_, err := adapter.Find(context.Background(), "/115/Media/a.mkv")
			if !errors.Is(err, tc.target) {
				t.Fatal(err)
			}
			if err.Error() == "SECRET" {
				t.Fatal("raw error leaked")
			}
		})
	}
}
func TestReadinessDirectoriesNoDirectURLAndListLimit(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		s := &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_Directory}
		a := testAdapter(t, s, Options{Token: "token"})
		if _, err := a.Resolve(context.Background(), storage.FileRef{Path: "/115"}); !errors.Is(err, storage.ErrDirectory) {
			t.Fatal(err)
		}
	})
	t.Run("no-direct", func(t *testing.T) {
		s := &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File, withoutURL: true}
		a := testAdapter(t, s, Options{Token: "token"})
		if _, err := a.Resolve(context.Background(), storage.FileRef{Path: "/115/a.mkv"}); !errors.Is(err, storage.ErrNoDirectURL) {
			t.Fatal("management static URL leaked as a direct URL", err)
		}
	})
	t.Run("not-ready", func(t *testing.T) {
		s := &protocolServer{notReady: true}
		a := testAdapter(t, s, Options{Token: "token"})
		if _, err := a.Probe(context.Background(), "/115"); !errors.Is(err, storage.ErrNotReady) {
			t.Fatal(err)
		}
	})
	t.Run("listing-limit", func(t *testing.T) {
		s := &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File}
		a := testAdapter(t, s, Options{Token: "token", MaxListFiles: 1})
		if _, err := a.List(context.Background(), "/115"); !errors.Is(err, storage.ErrLimitExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("expiration-absent", func(t *testing.T) {
		s := &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File, noExpiry: true}
		a := testAdapter(t, s, Options{Token: "token"})
		link, err := a.Resolve(context.Background(), storage.FileRef{Path: "/115/a.mkv"})
		if err != nil || !link.ExpiresAt.IsZero() {
			t.Fatal(err)
		}
	})
	t.Run("bad-login", func(t *testing.T) {
		s := &protocolServer{badLogin: true}
		a := testAdapter(t, s, Options{Username: "user", Password: "password"})
		if _, err := a.Find(context.Background(), "/115"); !errors.Is(err, storage.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("expired-token", func(t *testing.T) {
		s := &protocolServer{expiredLogin: true}
		a := testAdapter(t, s, Options{Username: "user", Password: "password"})
		if _, err := a.Find(context.Background(), "/115"); !errors.Is(err, storage.ErrInvalidResponse) {
			t.Fatal(err)
		}
	})
}
func TestEndpointValidation(t *testing.T) {
	for _, raw := range []string{"http://user:password@localhost:19798", "https://host/path", "https://host?token=secret", "dns:///host:19798", "http://localhost:0"} {
		if _, _, _, err := ParseEndpoint(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"localhost:19798", "http://127.0.0.1:19798", "https://cd.example.com"} {
		if _, _, _, err := ParseEndpoint(raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(Options{Endpoint: "http://localhost:19798"}); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if _, err := New(Options{Endpoint: "https://example.com", Token: "token", CAFile: "/nonexistent-ca-file"}); err == nil {
		t.Fatal("missing CA accepted")
	}
}
