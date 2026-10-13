package library

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestCloudHintsAreDurableScopedDebouncedAndNeverDeleteIndex(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var dbname string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&dbname); err != nil || dbname != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{})
	defer m.Close()
	c, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "owned change fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115/ChangesFixture"}, Secrets: map[string]string{"token": "owned-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	// Delete only fixture descendants; do not reset an unrelated DB or media row.
	defer func() {
		db.Exec(ctx, `DELETE FROM jobs WHERE idempotency_key LIKE 'library.scan:%' AND payload->>'scanId' IN (SELECT id::text FROM library_scans WHERE integration_id=$1)`, c.ID)
		db.Exec(ctx, `DELETE FROM library_scan_schedules WHERE integration_id=$1`, c.ID)
		db.Exec(ctx, `DELETE FROM library_scans WHERE integration_id=$1`, c.ID)
		db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, c.ID)
	}()
	e := Engine{DB: db, Connections: m}
	a, err := e.SaveSchedule(ctx, ScheduleConfig{Root: "/115/ChangesFixture/A", Enabled: true, Cron: "*/5 * * * *", WatchChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.SaveSchedule(ctx, ScheduleConfig{Root: "/115/ChangesFixture/B", Enabled: true, Cron: "*/5 * * * *", WatchChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	off, err := e.SaveSchedule(ctx, ScheduleConfig{Root: "/115/ChangesFixture/Off", Enabled: false, Cron: "*/5 * * * *", WatchChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	periodic, err := e.SaveSchedule(ctx, ScheduleConfig{Root: "/115/ChangesFixture/Periodic", Enabled: true, Cron: "*/5 * * * *"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE library_scan_schedules SET next_run_at=now()+interval '1 hour' WHERE integration_id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	read := func(id string) *time.Time {
		var v *time.Time
		if err := db.QueryRow(ctx, `SELECT change_scan_after FROM library_scan_schedules WHERE id=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	var assetsBefore, assetsAfter int64
	db.QueryRow(ctx, `SELECT count(*) FROM media_assets`).Scan(&assetsBefore)
	hint := clouddrive2.FileChange{Path: a.Root + "/one.mkv", NewPath: b.Root + "/renamed.mkv"}
	if err = e.recordChange(ctx, c.ID, c.Revision, &hint); err != nil {
		t.Fatal(err)
	}
	first := read(a.ID)
	if first == nil || read(b.ID) == nil || read(off.ID) != nil || read(periodic.ID) != nil {
		t.Fatal("scope or opt-in lost")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.recordChange(ctx, c.ID, c.Revision, &hint); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if !read(a.ID).Equal(*first) {
		t.Fatal("event storm postponed debounce")
	}
	unrelated := clouddrive2.FileChange{Path: "/115/OtherLibrary/other.mkv"}
	if err = e.recordChange(ctx, c.ID, c.Revision, &unrelated); err != nil {
		t.Fatal(err)
	}
	if err = e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM library_scans WHERE integration_id=$1`, c.ID).Scan(&count)
	if count != 0 {
		t.Fatal("debounce bypassed")
	}
	_, err = db.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=now()-interval '1 second' WHERE id IN ($1,$2)`, a.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Events received while a traversal is already active must cause a subsequent
	// scan, not be discarded by Start's idempotent existing-ID result.
	active, err := e.Start(ctx, a.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `SELECT count(*) FROM library_scans WHERE integration_id=$1`, c.ID).Scan(&count)
	if count != 2 || read(a.ID) == nil || read(b.ID) != nil {
		t.Fatal("in-flight dirty hint lost", count)
	}
	if _, err = db.Exec(ctx, `UPDATE library_scans SET state='succeeded' WHERE id=$1`, active); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=now()-interval '1 second' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if read(a.ID) != nil {
		t.Fatal("new scan did not consume hint")
	}
	var next string
	db.QueryRow(ctx, `SELECT last_scan_id::text FROM library_scan_schedules WHERE id=$1`, a.ID).Scan(&next)
	if next == active || next == "" {
		t.Fatal("did not admit genuinely new scan")
	}
	if err = e.recordChange(ctx, c.ID, c.Revision, &hint); err != nil {
		t.Fatal(err)
	}
	a.Enabled = false
	if _, err = e.SaveSchedule(ctx, a); err != nil {
		t.Fatal(err)
	}
	if read(a.ID) != nil {
		t.Fatal("disable retained event trigger")
	}
	if err = e.recordChange(ctx, c.ID, c.Revision, &hint); err != nil || read(a.ID) != nil {
		t.Fatal("stopped schedule received event")
	}
	if _, err = db.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=NULL WHERE integration_id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	current, err := m.Repository.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := m.Repository.Save(ctx, c.ID, integrations.Input{Kind: c.Kind, Name: c.Name, Enabled: true, Settings: current.Record.Settings, Revision: current.Record.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.recordChange(ctx, c.ID, c.Revision, &hint); err != nil || read(b.ID) != nil {
		t.Fatal("stale connection event applied")
	}
	// Reconnect lacks a server replay cursor; mark only opted-in current roots.
	if err = e.recordChange(ctx, c.ID, changed.Revision, nil); err != nil || read(b.ID) == nil || read(periodic.ID) != nil {
		t.Fatal("reconnect reconciliation absent")
	}
	// A changed configured root must not admit hints for historical scopes.
	currentRoot, _ := m.Repository.Get(ctx, c.ID)
	movedSettings := map[string]any{"endpoint": currentRoot.String("endpoint"), "cloudRoot": "/115/NewAllowedRoot"}
	moved, err := m.Repository.Save(ctx, c.ID, integrations.Input{Kind: c.Kind, Name: c.Name, Enabled: true, Settings: movedSettings, Revision: currentRoot.Record.Revision})
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=NULL WHERE integration_id=$1`, c.ID)
	if err = e.recordChange(ctx, c.ID, moved.Revision, nil); err != nil || read(b.ID) != nil {
		t.Fatal("old scope survived allowed-root edit")
	}
	db.QueryRow(ctx, `SELECT count(*) FROM media_assets`).Scan(&assetsAfter)
	if assetsAfter != assetsBefore {
		t.Fatal("push hint changed assets")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = e.recordChange(cancelled, c.ID, changed.Revision, &hint); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled write not stopped", err)
	}
}

func TestChangeTouchBoundsAndRenameAncestors(t *testing.T) {
	for _, tc := range []struct {
		root string
		h    clouddrive2.FileChange
		want bool
	}{
		{"/115/A", clouddrive2.FileChange{Path: "/115/A/file.mkv"}, true},
		{"/115/A", clouddrive2.FileChange{Path: "/115/A-other/file.mkv"}, false},
		{"/115/A", clouddrive2.FileChange{Path: "/115/elsewhere/a", NewPath: "/115/A/new"}, true},
		{"/115/A/subtree", clouddrive2.FileChange{Path: "/115/A", NewPath: "/115/Renamed", Directory: true}, true},
		{"/115/A", clouddrive2.FileChange{Path: "/115/A/../other"}, false},
	} {
		if got := changeTouches(tc.root, tc.h); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestUnsupportedPushDoesNotStartUnrequestedScanAndKeepsPeriodicSchedule(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var databaseName string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName)
	if databaseName != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, &fixtureCloud{})
	go server.Serve(listener)
	defer server.Stop()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{})
	defer m.Close()
	c, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "owned unsupported push fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "owned-test"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Exec(ctx, `DELETE FROM jobs WHERE payload->>'scanId' IN(SELECT id::text FROM library_scans WHERE integration_id=$1)`, c.ID)
		db.Exec(ctx, `DELETE FROM library_scan_schedules WHERE integration_id=$1`, c.ID)
		db.Exec(ctx, `DELETE FROM library_scans WHERE integration_id=$1`, c.ID)
		db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, c.ID)
	}()
	e := Engine{DB: db, Connections: m}
	if err = e.WatchWindow(ctx); err != nil {
		t.Fatal("no opt-in unexpectedly called RPC", err)
	}
	s, err := e.SaveSchedule(ctx, ScheduleConfig{Enabled: true, Cron: "*/5 * * * *", WatchChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.WatchWindow(ctx); !errors.Is(err, storage.ErrUnsupported) {
		t.Fatal("RPC capability lied", err)
	}
	list, err := e.Schedules(ctx)
	if err != nil || len(list) != 1 || list[0].WatchState != "unsupported" || list[0].ChangeScanAfter != nil {
		t.Fatal("unsupported stream created dirty hint", list, err)
	}
	db.Exec(ctx, `UPDATE library_scan_schedules SET next_run_at=now()-interval '1 minute' WHERE id=$1`, s.ID)
	if err = e.Schedule(ctx); err != nil {
		t.Fatal("periodic reconciliation stopped", err)
	}
	var scan string
	if err = db.QueryRow(ctx, `SELECT last_scan_id::text FROM library_scan_schedules WHERE id=$1`, s.ID).Scan(&scan); err != nil || scan == "" {
		t.Fatal("periodic scan not admitted", err)
	}
}
