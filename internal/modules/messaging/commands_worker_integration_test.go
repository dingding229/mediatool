package messaging

import (
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/adapters/telegram"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRealSameBinaryTelegramCommandAndSafeEvents(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("mandatory native AMD64 same-binary gate")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("dedicated non-root native host UID required")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	f := newIngressFixture(t, false)
	updates := []telegram.Update{}
	calls := 0
	f.transport(t, &updates, &calls)
	run, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() {
		done <- (pluginv2.Supervisor{Store: f.s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxWorkers: 1}).Run(run)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("reaper did not stop")
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
	wait("running", func() bool { _, e := f.s.Snapshot(f.ctx, f.instance.ID); return e == nil })
	updates = []telegram.Update{message(1, 10, 20, "/hello", f.i.now())}
	if _, e := f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	wait("real command result", func() bool {
		return f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command' AND state='succeeded'`) == 1
	})
	wait("targeted incoming event KV", func() bool {
		if e := f.s.Bridge(f.ctx); e != nil {
			t.Fatal(e)
		}
		if e := f.s.Deliver(f.ctx); e != nil {
			t.Fatal(e)
		}
		return f.count(t, `SELECT count(*) FROM plugin_v2_kv WHERE key LIKE 'acquisition-%'`) == 1
	})
	var id string
	var cipher []byte
	if e := f.s.DB.QueryRow(f.ctx, `SELECT id::text,ciphertext FROM plugin_v2_kv WHERE key='commands'`).Scan(&id, &cipher); e != nil {
		t.Fatal(e)
	}
	opened, e := f.s.Vault.Open("plugin:v2:kv:"+id, cipher)
	if e != nil || !json.Valid([]byte(opened["value"])) {
		t.Fatal(e)
	}

	signed, e := f.i.IssueAction(f.ctx, 10, 20, "hello", "")
	if e != nil {
		t.Fatal(e)
	}
	m := message(2, 10, 20, "", f.i.now())
	updates = []telegram.Update{{ID: 2, Callback: &telegram.Callback{ID: "real-fixture", From: telegram.Sender{ID: 20}, Message: m.Message, Data: signed}}}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	wait("actual signed callback command", func() bool {
		return f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command' AND state='succeeded'`) == 2
	})
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command'`) != 2 {
		t.Fatal("duplicate update routed")
	}
	instances, e := f.s.List(f.ctx, 0, 200)
	if e != nil || len(instances) != 1 {
		t.Fatal(e)
	}
	if _, e = f.s.Apply(f.ctx, "fixture", pluginv2.Request{Kind: pluginv2.Disable, InstanceID: f.instance.ID, ExpectedRevision: instances[0].Revision, Fingerprint: f.definition.Fingerprint, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	wait("stopped and reaped", func() bool {
		return f.count(t, `SELECT count(*) FROM plugin_v2_instances WHERE runtime_state='stopped' AND worker_pid=0 AND generation IS NULL`) == 1
	})
	updates = []telegram.Update{message(3, 10, 20, "/hello", f.i.now())}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command'`) != 2 {
		t.Fatal("disabled command routed")
	}
	t.Log("REAL_AMD64_F4D: fake Telegram -> allowlisted durable inbox -> real /hello Go handler + targeted event KV; duplicate/disabled rejected, worker reaped")
}
