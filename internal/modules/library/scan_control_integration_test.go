package library

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc"
	"io"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func scanWorkbenchFixture(t *testing.T) (context.Context, Engine, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	var settingsSnapshot []byte
	db.QueryRow(ctx, `SELECT value FROM application_settings WHERE singleton`).Scan(&settingsSnapshot)
	t.Cleanup(func() {
		db.Exec(context.Background(), `UPDATE application_settings SET value=$1 WHERE singleton`, settingsSnapshot)
	})
	cleanup := `TRUNCATE integrations,storage_connections,media_assets,jobs,outbox_events CASCADE`
	db.Exec(ctx, cleanup)
	t.Cleanup(func() { db.Exec(context.Background(), cleanup) })
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(srv, &fixtureCloud{})
	go srv.Serve(listener)
	t.Cleanup(func() { srv.Stop(); listener.Close() })
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{STRMAllowedRoots: []string{"/115/Media"}, CloudDriveRequestTimeout: time.Second})
	t.Cleanup(m.Close)
	c, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "Owned scan fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, Engine{DB: db, Connections: m}, c.ID
}
func TestScanPauseResumeGenerationCancelAndNoMissingVotes(t *testing.T) {
	ctx, e, connection := scanWorkbenchFixture(t)
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, connection)
	file := jobs.UUID()
	e.DB.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes)VALUES($1,$2,'old','/115/Media/old.mkv',1)`, file, connection)
	id, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.ControlScan(ctx, id, "pause"); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"scanId": id})
	if err = e.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal(err)
	}
	var votes int
	e.DB.QueryRow(ctx, `SELECT missing_scan_count FROM cloud_files WHERE id=$1`, file).Scan(&votes)
	if votes != 0 {
		t.Fatal("paused scan voted missing")
	}
	if err = e.ControlScan(ctx, id, "resume"); err != nil {
		t.Fatal(err)
	}
	if err = e.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("old generation ran", err)
	}
	fresh, _ := json.Marshal(map[string]any{"scanId": id, "generation": 2 + 1})
	if err = e.Process(ctx, jobs.Job{Payload: fresh, Attempts: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT missing_scan_count FROM cloud_files WHERE id=$1`, file).Scan(&votes)
	if votes != 1 {
		t.Fatal(votes)
	}
	scans, err := e.Scans(ctx)
	if err != nil || scans[0].CompletedDirectories != 1 || scans[0].Revision != 1 {
		t.Fatal(scans, err)
	}
	next, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	e.ControlScan(ctx, next, "cancel")
	payload, _ = json.Marshal(map[string]string{"scanId": next})
	if err = e.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT missing_scan_count FROM cloud_files WHERE id=$1`, file).Scan(&votes)
	if votes != 1 {
		t.Fatal("cancelled scan voted missing")
	}
}
func TestChangedConnectionAndReplacementCannotRebindAsset(t *testing.T) {
	ctx, e, connection := scanWorkbenchFixture(t)
	id, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, connection)
	raw, _ := json.Marshal(map[string]string{"scanId": id})
	if err = e.Process(ctx, jobs.Job{Payload: raw, Attempts: 3, MaxAttempts: 3}); err == nil {
		t.Fatal("stale scan succeeded")
	}
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, connection)
	original := storage.File{ID: "stable", Name: "old.mkv", Path: "/115/Media/old.mkv", Size: 10, Hashes: map[uint32]string{2: strings.Repeat("a", 40)}}
	file, err := e.indexFile(ctx, connection, original)
	if err != nil {
		t.Fatal(err)
	}
	candidate := jobs.UUID()
	e.DB.Exec(ctx, `INSERT INTO library_candidates(id,file_id)VALUES($1,$2)`, candidate, file)
	renamed := original
	renamed.Path = "/115/Media/renamed.mkv"
	renamed.Name = "renamed.mkv"
	same, err := e.indexFile(ctx, connection, renamed)
	if err != nil || same != file {
		t.Fatal("stable rename duplicated file", same, err)
	}
	var revision int
	e.DB.QueryRow(ctx, `SELECT revision FROM library_candidates WHERE id=$1`, candidate).Scan(&revision)
	if revision != 2 {
		t.Fatal("candidate change not invalidated", revision)
	}
	e.DB.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id,media_identity)VALUES($1,$1,'{}')`, file)
	replacement := renamed
	replacement.ID = "replacement"
	if _, err = e.indexFile(ctx, connection, replacement); err == nil {
		t.Fatal("old grant rebound to replacement")
	}
	var stable string
	e.DB.QueryRow(ctx, `SELECT provider_file_id FROM cloud_files WHERE id=$1`, file).Scan(&stable)
	if stable != "stable" {
		t.Fatal(stable)
	}
}

func TestCandidateRecognitionRequiresFreshFileAndUniqueExactIdentity(t *testing.T) {
	ctx, e, c := scanWorkbenchFixture(t)
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, c)
	original := storage.File{ID: "owned", Name: "Owned.Movie.2026.1080p.mkv", Path: "/115/Media/Owned.Movie.2026.1080p.mkv", Size: 1, Hashes: map[uint32]string{}}
	file, err := e.indexFile(ctx, c, original)
	if err != nil {
		t.Fatal(err)
	}
	id := jobs.UUID()
	e.DB.Exec(ctx, `INSERT INTO library_candidates(id,file_id)VALUES($1,$2)`, id, file)
	_, err = e.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "owned metadata", Enabled: true, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	e.Observe = func(context.Context, string, string) (storage.File, error) { return original, nil }
	duplicates := false
	e.Connections.OutboundHTTP = &http.Client{Transport: httpFixture(func(r *http.Request) (*http.Response, error) {
		body := `{"results":[{"id":101,"title":"Owned Movie","original_title":"Owned Movie","release_date":"2026-01-01"}]}`
		if duplicates {
			body = `{"results":[{"id":101,"title":"Owned Movie","release_date":"2026-01-01"},{"id":102,"title":"Owned Movie","release_date":"2026-01-01"}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	proof, err := e.Recognize(ctx, id)
	if err != nil || proof.Selected == nil || proof.Selected.ID != 101 || proof.FileDigest == "" {
		t.Fatal(proof, err)
	}
	duplicates = true
	// Clear only the isolated metadata cache by changing the connection revision.
	e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE kind='tmdb'`)
	proof, err = e.Recognize(ctx, id)
	if err != nil || proof.Selected != nil {
		t.Fatal("ambiguous identity preselected", proof, err)
	}
	original.ID = "replacement"
	if _, err = e.Recognize(ctx, id); err == nil {
		t.Fatal("replacement got old identity proof")
	}
}
