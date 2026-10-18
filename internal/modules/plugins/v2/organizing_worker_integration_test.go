package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRealSameBinaryOrganizingEventsAndTransferPolicies(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("mandatory native AMD64 release-worker CI gate")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("native host UID must match isolation gate")
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
	in, e := s.Apply(ctx, "test-admin", Request{Kind: Install, OperationKey: jobs.UUID(), PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), Config: json.RawMessage(`{"scheduled":false}`), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "test-admin", Request{Kind: Enable, OperationKey: jobs.UUID(), InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, Confirm: true})
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
			t.Error("worker cleanup timeout")
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
	wait("ready", func() bool { _, e := s.Snapshot(ctx, in.ID); return e == nil })
	a, b := jobs.UUID(), jobs.UUID()
	paths := []string{"Movies/demo.strm"}
	for _, typ := range []contracts.ChainEventType{contracts.ChainTransferRenameBuild, contracts.ChainTransferRename} {
		prepared, got, e := s.TransferPaths(ctx, typ, contracts.TransferPlan{OperationID: a, Paths: paths}, func(context.Context, []string) bool { return true })
		if e != nil || len(got) != 1 || got[0] != paths[0] {
			t.Fatal(typ, got, e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainTransferIntercept, contracts.ChainTransferOverwriteCheck} {
		prepared, e := s.TransferDecision(ctx, typ, contracts.TransferPlan{OperationID: a, Paths: paths})
		if e != nil {
			t.Fatal(e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
	}
	prepared, handle, e := s.SelectStorage(ctx, contracts.StorageSelection{Handles: []string{a}, Selected: a})
	if e != nil || handle != a {
		t.Fatal(handle, e)
	}
	if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
		t.Fatal(e)
	}
	var successfulChains int
	if e = s.DB.QueryRow(ctx, `SELECT count(DISTINCT handler) FROM plugin_v2_runs WHERE instance_id=$1 AND kind='chain' AND state='succeeded'`, in.ID).Scan(&successfulChains); e != nil || successfulChains != 5 {
		t.Fatal("all five policies must actually execute", successfulChains, e)
	}
	facts := map[string]any{
		"library.organize.failed":  contracts.OrganizeFailure{OperationID: a, Kind: "transfer", ReasonCode: "attempts_exhausted"},
		"library.subtitles.copied": contracts.SubtitleOutcome{OperationID: a, AssetID: b, Count: 1, State: "copied"},
		"library.subtitles.failed": contracts.SubtitleOutcome{OperationID: a, AssetID: b, Count: 1, State: "failed"},
		"library.history.hidden":   contracts.HistoryHidden{RecordID: a, Kind: "file"},
		"download.files.deleted":   contracts.DownloadDeleted{TaskID: b, FilesDeleted: true},
		"download.task.deleted":    contracts.DownloadDeleted{TaskID: b},
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	for typ, fact := range facts {
		if e = outbox.EmitVersionTx(ctx, tx, typ, a, "1", fact); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	wait("all six real worker receipts", func() bool {
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		var n int
		e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE 'acquisition-%'`, in.ID).Scan(&n)
		if e != nil {
			t.Fatal(e)
		}
		return n == 6
	})
	// Each receipt is a typed safe envelope, not a leaked business record.
	rows, e := s.DB.Query(ctx, `SELECT id::text,ciphertext FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE 'acquisition-%'`, in.ID)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var id string
		var cipher []byte
		if e = rows.Scan(&id, &cipher); e != nil {
			t.Fatal(e)
		}
		raw, e := s.open("kv", id, cipher)
		if e != nil || !json.Valid(raw) {
			t.Fatal(e)
		}
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	current, e := s.List(ctx, 0, 200)
	if e != nil || len(current) != 1 {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "test-admin", Request{Kind: Disable, InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: current[0].Revision, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	wait("disabled and reaped", func() bool {
		var stopped bool
		e := s.DB.QueryRow(ctx, `SELECT runtime_state='stopped' AND worker_pid=0 AND generation IS NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&stopped)
		return e == nil && stopped
	})
	if _, _, e = s.Descriptor(ctx, in.ID, "chain", "transfer.rename"); e == nil {
		t.Fatal("disabled hook callable")
	}
	t.Log("REAL_AMD64_F4B: six facts -> Go worker KV; five transfer/storage policies executed; disable fenced and reaped")
}
