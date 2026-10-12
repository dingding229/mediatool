// These are protocol fixtures, NOT a running 115 account or real Emby server.
package tests

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"media-tool/internal/adapters/clouddrive2"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
)

type cloudFixture struct {
	pb.UnimplementedCloudDriveFileSrvServer
	calls      atomic.Int32
	headerMode atomic.Bool
}

func (c *cloudFixture) GetSystemInfo(context.Context, *emptypb.Empty) (*pb.CloudDriveSystemInfo, error) {
	return &pb.CloudDriveSystemInfo{SystemReady: true, IsLogin: true}, nil
}
func (c *cloudFixture) FindFileByPath(ctx context.Context, r *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer cloud-test-token" {
		return nil, status.Error(codes.Unauthenticated, "secret upstream detail")
	}
	file := &pb.CloudDriveFile{Id: "cloud-id", Name: "movie.mkv", FullPathName: r.GetPath(), Size: 16, FileType: pb.CloudDriveFile_File}
	if r.GetPath() == "/115/Media" {
		file.Name = "Media"
		file.IsDirectory = true
	}
	return file, nil
}
func (c *cloudFixture) GetDownloadUrlPath(ctx context.Context, r *pb.GetDownloadUrlPathRequest) (*pb.DownloadUrlPathInfo, error) {
	if !r.GetGetDirectUrl() {
		return nil, status.Error(codes.InvalidArgument, "direct requested flag absent")
	}
	n := c.calls.Add(1)
	seconds := uint64(60)
	url := "https://cdn.fixture.example/media?ephemeral=" + strings.Repeat("x", int(n))
	reply := &pb.DownloadUrlPathInfo{DirectUrl: &url, ExpiresIn: &seconds}
	if c.headerMode.Load() {
		reply.UserAgent = proto.String("cloud-agent")
		reply.AdditionalHeaders = map[string]string{"Referer": "https://cloud.fixture.example/"}
	}
	return reply, nil
}
func TestProtocolFixtureSTRMCloudGRPCEmbyAndMediaFlow(t *testing.T) {
	ctx := context.Background()
	cloud := &cloudFixture{}
	cloud.headerMode.Store(true)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(grpcServer, cloud)
	go grpcServer.Serve(listener)
	defer grpcServer.Stop()
	defer listener.Close()
	adapter, err := clouddrive2.New(clouddrive2.Options{Endpoint: "http://" + listener.Addr().String(), Token: "cloud-test-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	var mediaCalls atomic.Int32
	media := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaCalls.Add(1)
		if r.UserAgent() != "cloud-agent" || r.Header.Get("Referer") == "" {
			t.Error("provider access conditions not applied")
		}
		if r.Header.Get("X-Emby-Token") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("caller credentials reached media provider")
		}
		if r.Header.Get("Range") != "bytes=4-11" {
			t.Error("Range not preserved")
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 4-11/16")
		w.Header().Set("Content-Length", "8")
		w.WriteHeader(206)
		if r.Method != "HEAD" {
			w.Write([]byte("456789ab"))
		}
	}))
	defer media.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, media.Listener.Addr().String())
	}}}
	policy := playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}
	engine := playback.New(adapter, playback.Options{Policy: policy, ProxyFallback: true, Client: client})
	app := httptest.NewUnstartedServer(nil)
	cfg := config.Config{Environment: "development", LegacyPlaybackEnabled: true, PublicURL: "http://" + app.Listener.Addr().String(), SessionKey: strings.Repeat("a", 64), STRMKey: strings.Repeat("b", 64), STRMAllowedRoots: []string{"/115/Media"}, CloudDriveEndpoint: "http://" + listener.Addr().String(), EmbyProxyEnabled: true, Emby302Enabled: true, STRMProxyFallback: true}
	builder := strm.Builder{BaseURL: cfg.PublicURL, APIKey: cfg.STRMKey, AllowedRoots: cfg.STRMAllowedRoots}
	stable, _ := builder.Build(storage.FileRef{Path: "/115/Media/movie.mkv"})
	originalCalls := atomic.Int32{}
	embyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			json.NewEncoder(w).Encode(map[string]string{"Id": "fixture", "Version": "fixture-only"})
		case "/Users/Me", "/Users/user-1":
			if r.Header.Get("X-Emby-Token") != "user-test-token" {
				w.WriteHeader(401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": "user-1", "Policy": map[string]bool{"EnableMediaPlayback": true}})
		case "/Users/user-1/Items/item-1":
			if r.Header.Get("X-Emby-Token") != "user-test-token" {
				w.WriteHeader(401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": "item-1", "PlayAccess": "Full", "MediaSources": []map[string]string{{"Id": "source-1", "Path": stable}}})
		default:
			originalCalls.Add(1)
			io.WriteString(w, "original-transcoding-path")
		}
	}))
	defer embyServer.Close()
	cfg.EmbyUpstream = embyServer.URL
	logs := &bytes.Buffer{}
	app.Config.Handler = httpapi.New(cfg, httpapi.Dependencies{Storage: adapter, Playback: engine, Logger: slog.New(slog.NewTextHandler(logs, nil))})
	app.Start()
	defer app.Close()
	caller := app.Client()
	caller.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, target, embyToken string) *http.Response {
		req, _ := http.NewRequest(method, target, nil)
		req.Header.Set("Range", "bytes=4-11")
		req.Header.Set("User-Agent", "player-agent")
		if embyToken != "" {
			req.Header.Set("X-Emby-Token", embyToken)
		}
		res, err := caller.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if probe, err := adapter.Probe(ctx, "/115/Media"); err != nil || !probe.PathReadable {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		res := request(method, stable, "")
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 206 || res.Header.Get("X-Media-Delivery") != "proxy" {
			t.Fatal(res.StatusCode, string(data))
		}
		if method == "GET" && string(data) != "456789ab" {
			t.Fatal(string(data))
		}
		if method == "HEAD" && len(data) != 0 {
			t.Fatal("HEAD streamed body")
		}
	}
	embyURL := app.URL + "/emby/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1"
	denied := request("GET", embyURL, "wrong-token")
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatal("Emby permission bypass", denied.StatusCode)
	}
	served := request("GET", embyURL, "user-test-token")
	data, _ := io.ReadAll(served.Body)
	served.Body.Close()
	if served.StatusCode != 206 || served.Header.Get("X-Media-Gateway") != "emby-static" || string(data) != "456789ab" {
		t.Fatal(served.StatusCode, string(data))
	}
	cloud.headerMode.Store(false)
	first := request("GET", stable, "")
	first.Body.Close()
	second := request("GET", stable, "")
	second.Body.Close()
	if first.StatusCode != 302 || second.StatusCode != 302 || first.Header.Get("Location") == second.Header.Get("Location") {
		t.Fatal("fixed link did not resolve fresh temporary addresses")
	}
	callsBefore := cloud.calls.Load()
	hls := request("GET", app.URL+"/emby/Videos/item-1/master.m3u8", "user-test-token")
	hls.Body.Close()
	if hls.StatusCode != 200 || cloud.calls.Load() != callsBefore || originalCalls.Load() != 1 {
		t.Fatal("transcoding path was intercepted")
	}
	if mediaCalls.Load() != 3 {
		t.Fatal("unexpected media reads", mediaCalls.Load())
	}
	// Inspect metadata is deliberately not a real-device playback assertion.
	inspection, err := engine.Inspect(context.Background(), storage.FileRef{Path: "/115/Media/movie.mkv"}, "player-agent")
	if err != nil || inspection.MediaBytesChecked {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), cfg.STRMKey) || !strings.Contains(logs.String(), "ephemeral=") {
		t.Fatal("raw request/302 logs missing")
	}
}
