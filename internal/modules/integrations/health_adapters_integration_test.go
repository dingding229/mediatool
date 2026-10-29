package integrations

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
)

type healthCloud struct {
	pb.UnimplementedCloudDriveFileSrvServer
	system, find atomic.Int32
}

func (s *healthCloud) GetSystemInfo(context.Context, *emptypb.Empty) (*pb.CloudDriveSystemInfo, error) {
	s.system.Add(1)
	return &pb.CloudDriveSystemInfo{IsLogin: true, SystemReady: true}, nil
}
func (s *healthCloud) FindFileByPath(ctx context.Context, in *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	s.find.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer test-token" || in.GetPath() != "/fixture" {
		return nil, status.Error(codes.Unauthenticated, "PRIVATE")
	}
	return &pb.CloudDriveFile{Name: "fixture", FullPathName: "/fixture", IsDirectory: true}, nil
}
func TestHealthCloudAndDownloaderOnlyReadAuthenticatedEndpoints(t *testing.T) {
	ctx, m, _ := healthFixture(t)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := grpc.NewServer()
	cloud := &healthCloud{}
	pb.RegisterCloudDriveFileSrvServer(server, cloud)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	var login, version atomic.Int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			login.Add(1)
			if r.Method != "POST" {
				t.Error("login method")
			}
			r.ParseForm()
			if r.Form.Get("username") != "user" || r.Form.Get("password") != "pass" {
				t.Error("authentication")
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "local-session", Path: "/"})
			w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			if r.Method != "GET" {
				t.Error("version must read")
			}
			if _, e := r.Cookie("SID"); e != nil {
				w.WriteHeader(403)
				return
			}
			version.Add(1)
			w.Write([]byte("v5.0.0"))
		default:
			t.Errorf("unexpected/mutating path %s", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer qb.Close()
	for _, in := range []Input{
		{Kind: "clouddrive2", Name: "cloud-heartbeat", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/fixture"}, Secrets: map[string]string{"token": "test-token"}},
		{Kind: "qbittorrent", Name: "qb-heartbeat", Enabled: true, Settings: map[string]any{"endpoint": qb.URL}, Secrets: map[string]string{"username": "user", "password": "pass"}},
	} {
		record, e := m.Repository.Save(ctx, "", in)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { m.Repository.DB.Exec(context.Background(), `DELETE FROM integrations WHERE id=$1`, record.ID) })
		if e = m.ScheduleHealth(ctx); e != nil {
			t.Fatal(e)
		}
		if !healthTick(t, ctx, m, record.ID, nil).Passed {
			t.Fatal(in.Kind)
		}
		var delay float64
		m.Repository.DB.QueryRow(ctx, `SELECT extract(epoch FROM(next_check_at-clock_timestamp())) FROM integration_health_schedule WHERE integration_id=$1`, record.ID).Scan(&delay)
		if delay < 59 || delay > 76 {
			t.Fatal(delay)
		}
	}
	if cloud.system.Load() != 1 || cloud.find.Load() != 1 || login.Load() != 1 || version.Load() != 1 {
		t.Fatal("unexpected probe calls", cloud.system.Load(), cloud.find.Load(), login.Load(), version.Load())
	}
}
func TestHealthSaveArmsImmediateScheduleAndWake(t *testing.T) {
	ctx, m, r := healthFixture(t)
	m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()+interval '6 hours' WHERE integration_id=$1`, r.ID)
	record, e := m.SaveProbed(ctx, r.ID, Input{Kind: r.Kind, Name: r.Name, Enabled: true, Settings: r.Settings, Revision: r.Revision}, nil)
	if e != nil {
		t.Fatal(e)
	}
	var due bool
	m.Repository.DB.QueryRow(ctx, `SELECT next_check_at<=clock_timestamp() FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&due)
	if !due {
		t.Fatal("save deferred to old schedule")
	}
	select {
	case <-m.healthWake:
	case <-time.After(time.Second):
		t.Fatal("no immediate wake")
	}
	if record.Revision != r.Revision+1 {
		t.Fatal("save revision")
	}
}
func TestHealthLoopFirstProbeAndShutdownWithoutWaitingForTicker(t *testing.T) {
	ctx, m, r := healthFixture(t)
	reached := make(chan struct{}, 1)
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("probe lacks ten-second bound")
		}
		select {
		case reached <- struct{}{}:
		default:
		}
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	loop, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); m.RunHealth(loop, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("startup waited for ticker")
	}
	deadline := time.Now().Add(time.Second)
	for {
		c, e := m.Repository.Get(ctx, r.ID)
		if e == nil && c.Record.LastCheckState == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("missing startup receipt")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("health loop not joined")
	}
}
