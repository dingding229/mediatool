package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type fixtureCloud struct {
	pb.UnimplementedCloudDriveFileSrvServer
	mu    sync.Mutex
	files []*pb.CloudDriveFile
	fail  bool
}

func (c *fixtureCloud) FindFileByPath(ctx context.Context, r *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	if r.Path == "/115/Media" {
		return &pb.CloudDriveFile{Id: "root", Name: "Media", FullPathName: r.Path, IsDirectory: true}, nil
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (c *fixtureCloud) GetSubFiles(r *pb.ListSubFileRequest, stream grpc.ServerStreamingServer[pb.SubFilesReply]) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return status.Error(codes.Unavailable, "fixture unavailable")
	}
	for _, file := range c.files {
		if err := stream.Send(&pb.SubFilesReply{SubFiles: []*pb.CloudDriveFile{file}}); err != nil {
			return err
		}
	}
	return nil
}

type httpFixture func(*http.Request) (*http.Response, error)

func (f httpFixture) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostgresCloudScanConfirmationFileGrantAndMissingSafety(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated test DB required")
	}
	cleanup := `TRUNCATE subscriptions,integrations,cloud_files,storage_connections,media_items,media_assets,jobs,outbox_events CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	file := &pb.CloudDriveFile{Id: "stable-cloud-file", Name: "Owned.Movie.2026.mkv", FullPathName: "/115/Media/Owned.Movie.2026.mkv", FileType: pb.CloudDriveFile_File, Size: 100, FileHashes: map[uint32]string{2: strings.Repeat("a", 40)}}
	cloud := &fixtureCloud{files: []*pb.CloudDriveFile{file}}
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, cloud)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	output := t.TempDir()
	manager := integrations.NewManager(repo, config.Config{PublicURL: "https://media.fixture.example", STRMKey: strings.Repeat("b", 64), STRMOutputDir: output, STRMAllowedRoots: []string{"/115/Media"}, CloudDriveRequestTimeout: time.Second})
	defer manager.Close()
	manager.OutboundHTTP = &http.Client{Transport: httpFixture(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":1,"title":"Owned Movie","original_title":"Owned Movie","release_date":"2026-01-01","overview":"Owned test fixture"}`))}, nil
	})}
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "own cloud fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "own-test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "own metadata", Enabled: true, Settings: map[string]any{}, Secrets: map[string]string{"token": "own-tmdb"}}); err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Connections: manager}
	scanID, err := engine.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"scanId": scanID})
	if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	candidates, err := engine.Candidates(ctx)
	if err != nil || len(candidates) != 1 {
		t.Fatal(candidates, err)
	}
	for _, state := range []string{"needs_confirmation", "pending"} {
		page, err := engine.CandidatePage(ctx, 50, 0, state)
		if err != nil || len(page) != 1 {
			t.Fatal("pending candidate state filter lost a real indexed file", state, err)
		}
	}
	assetID, err := engine.Confirm(ctx, candidates[0].ID, Confirmation{MediaType: "movie", TMDBID: 1, Revision: candidates[0].Revision})
	if err != nil {
		t.Fatal(err)
	}
	grants := playbackgrants.Service{DB: db, Vault: vault, PublicURL: manager.Config.PublicURL}
	link, err := grants.ExistingURL(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := grants.VerifyManagedURL(ctx, link)
	if err != nil || ref.Path != file.FullPathName || ref.AssetID != assetID {
		t.Fatal(ref, err)
	}
	var strmPath string
	if err = db.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, assetID).Scan(&strmPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, strmPath))
	if err != nil || !strings.Contains(string(raw), "assetId=") {
		t.Fatal("file scoped STRM not generated", err)
	}
	if _, err = os.Stat(filepath.Join(output, strings.TrimSuffix(strmPath, ".strm")+".nfo")); err != nil {
		t.Fatal("NFO missing", err)
	}

	// Paginated candidate views are independent of full scan coverage.
	paged, err := engine.CandidatePage(ctx, 1, 0, "confirmed")
	if err != nil || len(paged) != 1 {
		t.Fatal("candidate paging", err)
	}
	if err = engine.Correct(ctx, assetID, Confirmation{MediaType: "movie", TMDBID: 1, Revision: 1}); err != nil {
		t.Fatal("identity correction", err)
	}
	if err = engine.Correct(ctx, assetID, Confirmation{MediaType: "movie", TMDBID: 1, Revision: 1}); err == nil {
		t.Fatal("stale identity revision accepted")
	}
	after, _ := grants.ExistingURL(ctx, assetID)
	if after != link {
		t.Fatal("identity edit rotated playback grant")
	}
	schedule, err := engine.SaveSchedule(ctx, ScheduleConfig{Cron: "*/5 * * * *"})
	if err != nil || schedule.Enabled {
		t.Fatal("schedule not default off", err)
	}
	if err = engine.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var before int
	db.QueryRow(ctx, `SELECT count(*) FROM library_scans`).Scan(&before)
	schedule.Enabled = true
	schedule, err = engine.SaveSchedule(ctx, schedule)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `UPDATE library_scan_schedules SET next_run_at=now()-interval '1 minute' WHERE id=$1`, schedule.ID)
	if err = engine.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if err = engine.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var scheduled string
	db.QueryRow(ctx, `SELECT last_scan_id::text FROM library_scan_schedules WHERE id=$1`, schedule.ID).Scan(&scheduled)
	if scheduled == "" {
		t.Fatal("scheduled scan missing")
	}
	if err = engine.Process(ctx, jobs.Job{Payload: json.RawMessage(`{"scanId":"` + scheduled + `"}`), Attempts: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	schedule.Enabled = false
	if _, err = engine.SaveSchedule(ctx, schedule); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	cloud.files = nil
	cloud.fail = true
	cloud.mu.Unlock()
	failed, _ := engine.Start(ctx, "")
	payload, _ = json.Marshal(map[string]string{"scanId": failed})
	if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 3, MaxAttempts: 3}); err == nil {
		t.Fatal("failed directory claimed complete")
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM cloud_files WHERE id=$1`, assetID).Scan(&state)
	if state != "present" {
		t.Fatal("incomplete scan marked missing", state)
	}
	cloud.mu.Lock()
	cloud.fail = false
	cloud.mu.Unlock()
	for n := 0; n < 2; n++ {
		id, err := engine.Start(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		payload, _ = json.Marshal(map[string]string{"scanId": id})
		if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); err != nil {
			t.Fatal(err)
		}
	}
	db.QueryRow(ctx, `SELECT state FROM cloud_files WHERE id=$1`, assetID).Scan(&state)
	if state != "missing" {
		t.Fatal("missing confirmation failed", state)
	}
	if _, err = grants.VerifyManagedURL(ctx, link); !errors.Is(err, playbackgrants.ErrDenied) {
		t.Fatal("missing file remained playable")
	}
	if _, err = os.Stat(filepath.Join(output, strmPath)); err != nil {
		t.Fatal("scan deleted STRM")
	}
	cloud.mu.Lock()
	copyFile := proto.Clone(file).(*pb.CloudDriveFile)
	copyFile.Name = "Moved.Movie.2026.mkv"
	copyFile.FullPathName = "/115/Media/Moved.Movie.2026.mkv"
	cloud.files = []*pb.CloudDriveFile{copyFile}
	cloud.mu.Unlock()
	id, _ := engine.Start(ctx, "")
	payload, _ = json.Marshal(map[string]string{"scanId": id})
	if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	ref, err = grants.VerifyManagedURL(ctx, link)
	if err != nil || ref.Path != copyFile.FullPathName {
		t.Fatal("stable link did not follow validated cloud move", ref, err)
	}
	if err = grants.Revoke(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if _, err = grants.VerifyManagedURL(ctx, link); !errors.Is(err, playbackgrants.ErrDenied) {
		t.Fatal("revoked grant accepted")
	}
	newLink, err := grants.Rotate(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = grants.VerifyManagedURL(ctx, link); !errors.Is(err, playbackgrants.ErrDenied) {
		t.Fatal("old rotated key accepted")
	}
	if _, err = grants.VerifyManagedURL(ctx, newLink); err != nil {
		t.Fatal(err)
	}
}
