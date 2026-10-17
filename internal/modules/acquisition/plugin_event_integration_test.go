package acquisition

import (
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// This is an actual downloader Add -> committed fact -> bridge -> Go worker
// test. Only the torrent/115 transport is fake; no production media is touched.
func TestPluginRealBinaryDownloadAdded(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("native same-binary worker kernel gate required")
	}
	if err := sandbox.Parent(); err != nil {
		t.Fatal(err)
	}
	f := newSourceFixture(t)
	s := pluginv2.Store{DB: f.engine.DB, Vault: f.engine.Connections.Repository.Vault, Root: filepath.Join(t.TempDir(), "plugins")}
	defs, err := pluginv2.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d := defs[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	instance, err := s.Apply(f.ctx, "fixture-admin", pluginv2.Request{Kind: pluginv2.Install, OperationKey: jobs.UUID(), PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes, Config: json.RawMessage(`{"scheduled":false}`), Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	instance, err = s.Apply(f.ctx, "fixture-admin", pluginv2.Request{Kind: pluginv2.Enable, InstanceID: instance.ID, ExpectedRevision: instance.Revision, OperationKey: jobs.UUID(), Fingerprint: d.Fingerprint, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() {
		done <- (pluginv2.Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxWorkers: 1}).Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker cleanup timeout")
		}
		bounded, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, table := range []string{"manual_requests", "api_keys", "api_limits", "runs", "subscriptions", "schedules", "extensions", "snapshots", "operations"} {
			if _, err := s.DB.Exec(bounded, "DELETE FROM plugin_v2_"+table+" WHERE instance_id=$1", instance.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := s.DB.Exec(bounded, `DELETE FROM plugin_v2_instances WHERE id=$1`, instance.ID); err != nil {
			t.Error(err)
		}
	})
	wait := func(label string, fn func() bool) {
		t.Helper()
		end := time.Now().Add(10 * time.Second)
		for time.Now().Before(end) {
			if fn() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		rows, err := s.List(context.Background(), 0, 200)
		t.Fatal(label, rows, err)
	}
	wait("descriptor ready", func() bool { _, err := s.Snapshot(f.ctx, instance.ID); return err == nil })
	args, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: args}); err != nil {
		t.Fatal(err)
	}
	// Retry the business command; the already-downloading fence must not emit twice.
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: args}); err != nil {
		t.Fatal(err)
	}
	var event string
	var n int
	if err = s.DB.QueryRow(f.ctx, `SELECT count(*),min(id::text) FROM outbox_events WHERE event_type='download.started' AND aggregate_id=$1`, f.taskID).Scan(&n, &event); err != nil || n != 1 {
		t.Fatal("fact dedupe", n, err)
	}
	wait("DownloadAdded received", func() bool {
		data, err := s.Inspect(f.ctx, instance.ID, "")
		if err != nil {
			return false
		}
		for _, entry := range data.Entries {
			if entry.Key == "download-"+event {
				return true
			}
		}
		return false
	})
	t.Log("ACTUAL native worker DownloadAdded E2E: fake downloader Add -> transaction outbox -> Go handler -> KV; duplicate Add is idempotent")
}
func TestDownloadStartedDistinctFactsOnSameTask(t *testing.T) {
	f := newSourceFixture(t)
	_, _, _, _, details, err := f.engine.load(f.ctx, f.taskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"downloading", "downloading", "local_complete", "downloading"} {
		event := "download.started"
		if state == "local_complete" {
			event = ""
		}
		if err = f.engine.transition(f.ctx, f.taskID, state, event, details); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='download.started'`, f.taskID).Scan(&count); err != nil || count != 2 {
		t.Fatal("second fact swallowed", count, err)
	}
}
