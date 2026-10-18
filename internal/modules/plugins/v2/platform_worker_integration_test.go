package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/testworker"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRealSameBinaryPlatformFactsAndPreEffectDeclines(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("mandatory native AMD64 static worker gate")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("native host UID")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	s := fixture(t)
	ctx := context.Background()
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	in, e := s.Apply(ctx, "test", Request{Kind: Install, PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), OperationKey: jobs.UUID(), Confirm: true, Config: json.RawMessage(`{"scheduled":false}`)})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "test", Request{Kind: Enable, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- (Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxWorkers: 1}).Run(run)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker reaper timeout")
		}
	})
	wait := func(name string, fn func() bool) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if fn() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal(name)
	}
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, err := s.UI(c, in.ID, "form", ""); return err })

	notification := jobs.UUID()
	for _, kind := range []string{"notification", "asset"} {
		module := "notifications.channel"
		if kind == "asset" {
			module = "mediaserver.refresh"
		}
		receipt, e := s.ExternalEffect(ctx, module, contracts.EffectRequest{RequestID: jobs.UUID(), ObjectID: notification, Kind: kind})
		if e != nil || receipt.State != "not_started" {
			t.Fatal("pure offer must decline without execute", receipt, e)
		}
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='module' AND state='succeeded'`).Scan(&n); e != nil || n != 2 {
		t.Fatal("two actual offer handlers", n, e)
	}
	facts := map[string]any{
		"notification.queued":       contracts.NotificationQueued{NotificationID: notification, Origin: "builtin"},
		"system.operation.failed":   contracts.OperationFailed{Source: "jobs", Code: "terminal_failure", Count: 2, Window: time.Now().UTC().Format(time.RFC3339)},
		"settings.changed":          contracts.SettingsChanged{Area: "application", Revision: 2, Changed: []string{"searchCacheSeconds"}},
		"plugin.config.changed":     contracts.PluginConfigChanged{InstanceID: in.ID, Revision: 2},
		"messaging.inbound":         contracts.InboundMessage{InstanceID: in.ID, Command: "hello", Actor: "opaque", Kind: "command"},
		"messaging.action.received": contracts.InboundMessage{InstanceID: in.ID, Command: "hello", Actor: "opaque", Kind: "action"},
	}

	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	for typ, fact := range facts {
		if e = outbox.EmitVersionTx(ctx, tx, typ, in.ID, "v1", fact); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	wait("six committed platform facts in real Go worker KV", func() bool {
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE 'acquisition-%'`, in.ID).Scan(&n)
		return e == nil && n == 6
	})
	current, e := s.List(ctx, 0, 200)
	if e != nil || len(current) != 1 {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "test", Request{Kind: Disable, InstanceID: in.ID, ExpectedRevision: current[0].Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	wait("disabled/reaped", func() bool {
		var stopped bool
		e = s.DB.QueryRow(ctx, `SELECT runtime_state='stopped' AND worker_pid=0 AND generation IS NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&stopped)
		return e == nil && stopped
	})
	if _, _, e = s.Descriptor(ctx, in.ID, "module", "notifications.channel"); e == nil {
		t.Fatal("disabled source callable")
	}
	t.Log("REAL_AMD64_F4D_PLATFORM: six committed platform facts -> real Go worker KV; two pure offer declines, no execute RPC; disabled and reaped")
}
