package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/secrets"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "plugin-worker" {
		if e := Worker(os.Args[2:]); e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestCatalogIsPureAndPathsRejectSymlinks(t *testing.T) {
	all, e := Catalog()
	if e != nil || len(all) != 1 {
		t.Fatal(e)
	}
	all[0].Manifest.Permissions[0].Scope = "system.exec"
	again, e := Catalog()
	if e != nil || again[0].Manifest.Permissions[0].Scope == "system.exec" {
		t.Fatal("shared metadata")
	}
	s := Store{Root: filepath.Join(t.TempDir(), "plugins")}
	id := jobs.UUID()
	if _, e := s.DataPath(id); e == nil {
		t.Fatal("unselected instance has data path")
	}
	if e = s.EnsurePaths(id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataPath(id); e != nil {
		t.Fatal(e)
	}
	other := t.TempDir()
	if e = os.Remove(filepath.Join(s.Root, id, "data")); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(other, filepath.Join(s.Root, id, "data")); e != nil {
		t.Fatal(e)
	}
	if e = s.EnsurePaths(id); e == nil {
		t.Fatal("data symlink allowed")
	}
	for _, args := range [][]string{{}, {"--role", "all"}, {"--plugin", "hello-go", "--instance", id, "--instance", id}} {
		if Worker(args) == nil {
			t.Fatal("invalid worker command allowed")
		}
	}
}
func fixture(t *testing.T) Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("only media_tool_test is allowed")
	}
	schema := "plugin_g2_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	base, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	if _, e = base.Exec(context.Background(), `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	// Copy only the G2 tables into a disposable schema; serial migration tests establish their source schema.
	tables := []string{"plugin_v2_definitions", "plugin_v2_instances", "plugin_v2_configs", "plugin_v2_kv", "plugin_v2_grants", "plugin_v2_operations", "audit_logs", "outbox_events", "jobs", "plugin_v2_snapshots", "plugin_v2_extensions", "plugin_v2_schedules", "plugin_v2_event_journal", "plugin_v2_subscriptions", "plugin_v2_runs", "plugin_v2_api_keys", "plugin_v2_api_limits", "plugin_v2_manual_requests"}
	for _, table := range tables {
		if _, e = base.Exec(context.Background(), `CREATE TABLE `+pgx.Identifier{schema, table}.Sanitize()+` (LIKE public.`+table+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e = pool.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); e != nil {
			t.Error(e)
		}
		pool.Close()
	})
	vault, e := secrets.New(strings.Repeat("33", 32))
	if e != nil {
		t.Fatal(e)
	}
	return Store{DB: pool, Vault: vault, Root: filepath.Join(t.TempDir(), "plugins")}
}
func TestRealSameBinaryWorkerWritesKVAndStops(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real worker requires mandatory Linux AMD64 gate")
	}
	s := fixture(t)
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	req := Request{Kind: Install, OperationKey: "real-worker-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d), Config: json.RawMessage(`{}`)}
	first, e := s.Apply(context.Background(), "test-admin", req)
	if e != nil {
		t.Fatal(e)
	}
	req = Request{Kind: Enable, OperationKey: "real-worker-enable", InstanceID: first.ID, ExpectedRevision: first.Revision, Fingerprint: d.Fingerprint, Confirm: true}
	enabled, e := s.Apply(context.Background(), "test-admin", req)
	if e != nil {
		t.Fatal(e)
	}
	exe := os.Getenv("MEDIA_TOOL_PLUGIN_BINARY")
	if exe == "" {
		exe, e = os.Executable()
		if e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	super := Supervisor{Store: s, Executable: exe, Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	done := make(chan error, 1)
	go func() { done <- super.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(20 * time.Second):
			t.Error("supervisor cleanup hung")
		}
	})
	var id string
	var cipher []byte
	deadline := time.Now().Add(25 * time.Second)
	for {
		e = s.DB.QueryRow(context.Background(), `SELECT id::text,ciphertext FROM plugin_v2_kv WHERE instance_id=$1 AND key='starts'`, first.ID).Scan(&id, &cipher)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			rows, _ := s.List(context.Background(), 0, 200)
			t.Fatalf("hello did not write KV: %v states=%v", e, rows)
		}
		time.Sleep(100 * time.Millisecond)
	}
	value, e := s.open("kv", id, cipher)
	if e != nil || string(value) != "1" {
		t.Fatalf("KV=%s err=%v", value, e)
	}
	var workerPID int
	for {
		if e = s.DB.QueryRow(context.Background(), `SELECT worker_pid FROM plugin_v2_instances WHERE id=$1 AND runtime_state='running'`, first.ID).Scan(&workerPID); e == nil && workerPID > 0 {
			break
		}
		if time.Now().After(deadline) {
			rows, err := s.List(context.Background(), 0, 200)
			t.Fatal("worker not running", rows, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if e = sandbox.KillGroup(workerPID); e != nil {
		t.Fatal(e)
	}
	for {
		if e = s.DB.QueryRow(context.Background(), `SELECT id::text,ciphertext FROM plugin_v2_kv WHERE instance_id=$1 AND key='starts'`, first.ID).Scan(&id, &cipher); e == nil {
			value, e = s.open("kv", id, cipher)
			if e == nil && string(value) == "2" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("crash did not converge/restart", e, string(value))
		}
		time.Sleep(100 * time.Millisecond)
	}
	// A second owner may only replace an expired lease AFTER the old PID/start
	// reference has been reaped. Sample the kernel to catch overlapping workers.
	var oldPID int
	var oldStart string
	for {
		e = s.DB.QueryRow(context.Background(), `SELECT worker_pid,worker_start FROM plugin_v2_instances WHERE id=$1 AND runtime_state='running'`, first.ID).Scan(&oldPID, &oldStart)
		if e == nil && oldPID > 0 {
			break
		}
		if time.Now().After(deadline) {
			rows, err := s.List(context.Background(), 0, 200)
			t.Fatal("restarted worker not running", rows, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	otherCtx, otherStop := context.WithCancel(ctx)
	otherDone := make(chan error, 1)
	go func() { otherDone <- super.Run(otherCtx) }()
	t.Cleanup(func() {
		otherStop()
		select {
		case e := <-otherDone:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(20 * time.Second):
			t.Error("second owner did not stop")
		}
	})
	if _, e = s.DB.Exec(context.Background(), `UPDATE plugin_v2_instances SET lease_until=now()-interval '1 second' WHERE id=$1`, first.ID); e != nil {
		t.Fatal(e)
	}
	deadline = time.Now().Add(25 * time.Second)
	for {
		var currentPID int
		if e = s.DB.QueryRow(context.Background(), `SELECT worker_pid FROM plugin_v2_instances WHERE id=$1`, first.ID).Scan(&currentPID); e != nil {
			t.Fatal(e)
		}
		if currentPID > 0 && currentPID != oldPID {
			all, e := sandbox.Snapshot()
			if e != nil {
				t.Fatal(e)
			}
			for _, p := range all {
				if p.PID == oldPID && p.Start == oldStart {
					t.Fatal("new owner started before old process was reaped")
				}
			}
		}
		if e = s.DB.QueryRow(context.Background(), `SELECT id::text,ciphertext FROM plugin_v2_kv WHERE instance_id=$1 AND key='starts'`, first.ID).Scan(&id, &cipher); e == nil {
			value, e = s.open("kv", id, cipher)
			if e == nil && string(value) == "3" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("expired owner did not recover", e, string(value))
		}
		time.Sleep(100 * time.Millisecond)
	}
	req = Request{Kind: Disable, OperationKey: "real-worker-disable", InstanceID: first.ID, ExpectedRevision: enabled.Revision, Fingerprint: d.Fingerprint, Confirm: true}
	if _, e = s.Apply(context.Background(), "test-admin", req); e != nil {
		t.Fatal(e)
	}
	for {
		var state State
		if e = s.DB.QueryRow(context.Background(), `SELECT runtime_state FROM plugin_v2_instances WHERE id=$1`, first.ID).Scan(&state); e != nil {
			t.Fatal(e)
		}
		if state == Stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disable did not reap worker", state)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("real same-binary hello initialized, encrypted KV written, crash restarted once, expired-owner failover did not overlap, disable reaped worker")
}
func TestHostRejectsUnknownAndAuthorityFromParameters(t *testing.T) {
	s := fixture(t)
	l := Lease{ID: jobs.UUID(), Token: jobs.UUID(), Owner: jobs.UUID()}
	h := s.Host(l)
	for _, raw := range []string{`{"method":"sql.query","args":{}}`, `{"method":"plugin.data.get","args":{"key":"x"},"instanceId":"other"}`} {
		_, e := h(context.Background(), json.RawMessage(raw))
		if e == nil {
			t.Fatal("forged Host request accepted")
		}
	}
	_, e := s.List(context.Background(), 0, 201)
	if !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	_ = contracts.ContractVersion
}

func TestEnvironmentIsExplicitAndContainsNoAppSecrets(t *testing.T) {
	for _, name := range []string{"APP_SECRET_KEY", "SESSION_SECRET_KEY", "DATABASE_URL", "REDIS_URL", "STRM_API_KEY", "CLOUDDRIVE_TOKEN", "HTTP_PROXY"} {
		t.Setenv(name, "private-synthetic-value")
	}
	env := workerEnvironment("/data/plugins/fixture", "fixture")
	if len(env) != 8 {
		t.Fatal("unexpected inherited variables", len(env))
	}
	for _, entry := range env {
		if strings.Contains(entry, "private-synthetic-value") {
			t.Fatal("app secret inherited")
		}
	}
}

func TestStderrBoundAndKnownWarningWithoutRawLeak(t *testing.T) {
	var warned int
	tail := &stderrLimit{exceeded: make(chan struct{}), onLandlock: func() { warned++ }}
	for _, s := range []string{"warning reason=landlock_", "unavailable private-synthetic-value"} {
		if _, e := tail.Write([]byte(s)); e != nil {
			t.Fatal(e)
		}
	}
	if warned != 1 {
		t.Fatal("known warning not reported")
	}
	if _, e := tail.Write([]byte(strings.Repeat("x", 70000))); e != nil {
		t.Fatal(e)
	}
	if len(tail.tail) > 4096 {
		t.Fatal("unbounded stderr")
	}
	select {
	case <-tail.exceeded:
	default:
		t.Fatal("output limit did not trip")
	}
}

func TestAuthorizedCoreHostDispatcher(t *testing.T) {
	s := fixture(t)
	d, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	def := d[0]
	req := Request{Kind: Install, OperationKey: "core-host-install", PluginID: def.Manifest.ID, Fingerprint: def.Fingerprint, Confirm: true, Scopes: scopes(def), Config: json.RawMessage(`{"greeting":"first"}`)}
	in, e := s.Apply(context.Background(), "test-admin", req)
	if e != nil {
		t.Fatal(e)
	}
	req = Request{Kind: Enable, OperationKey: "core-host-enable", InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: def.Fingerprint, Confirm: true}
	if _, e = s.Apply(context.Background(), "test-admin", req); e != nil {
		t.Fatal(e)
	}
	l, e := s.Claim(context.Background(), jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	for _, scope := range []string{"plugin.config.write", "notifications.send"} {
		if _, e = s.DB.Exec(context.Background(), `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3) ON CONFLICT(instance_id,scope)DO NOTHING`, l.ID, scope, l.Fingerprint); e != nil {
			t.Fatal(e)
		}
	}
	call := func(method string, args any, rev *int64) (any, error) {
		t.Helper()
		tx, e := s.DB.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		defer rollback(context.Background(), tx)
		raw, e := json.Marshal(args)
		if e != nil {
			t.Fatal(e)
		}
		return s.authorizedCall(context.Background(), tx, l, contracts.HostCall{Method: method, Args: raw, ExpectedRevision: rev}, "running")
	}
	config, e := call("plugin.config.get", struct{}{}, nil)
	if e != nil || config.(contracts.Config).Revision != 1 {
		t.Fatal(e, config)
	}
	path, e := call("plugin.data.path", struct{}{}, nil)
	if e != nil || !strings.Contains(fmt.Sprint(path), in.ID) {
		t.Fatal(e, path)
	}
	if _, e = call("plugin.state.get", struct{}{}, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = call("system.summary", struct{}{}, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = call("notifications.send", contracts.Message{Title: "synthetic", Text: "body"}, nil); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE event_type='plugin.notice' AND aggregate_id=$1`, in.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(e, n)
	}
	if _, e = call("plugin.data.list", map[string]any{"after": "", "limit": 200}, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = call("plugin.data.list", map[string]any{"after": "", "limit": 201}, nil); e == nil {
		t.Fatal("unbounded KV list")
	}
	one := int64(1)
	value, e := call("plugin.config.update", map[string]any{"value": map[string]string{"greeting": "second"}}, &one)
	if e != nil || value.(contracts.Revision).Revision != 2 {
		t.Fatal(e, value)
	}
	if _, e = call("plugin.config.update", map[string]any{"value": map[string]string{}}, &one); e == nil {
		t.Fatal("stale config write")
	}
	var grant int64
	if e = s.DB.QueryRow(context.Background(), `SELECT grant_revision FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&grant); e != nil || grant <= l.GrantRevision {
		t.Fatal("config change did not fence old generation", grant, e)
	}
	if _, e = s.hostCall(context.Background(), l, json.RawMessage(`{"method":"plugin.state.get","args":{}}`)); e == nil {
		t.Fatal("old worker remained authorized after config update")
	}

}

func TestLinuxWorkerRSSLimitConverges(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux release RSS gate required")
	}
	s := fixture(t)
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	req := Request{Kind: Install, OperationKey: "rss-cap-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d), Config: json.RawMessage(`{}`)}
	first, e := s.Apply(context.Background(), "test-admin", req)
	if e != nil {
		t.Fatal(e)
	}
	req = Request{Kind: Enable, OperationKey: "rss-cap-enable", InstanceID: first.ID, ExpectedRevision: first.Revision, Fingerprint: d.Fingerprint, Confirm: true}
	if _, e = s.Apply(context.Background(), "test-admin", req); e != nil {
		t.Fatal(e)
	}
	exe := os.Getenv("MEDIA_TOOL_PLUGIN_BINARY")
	if exe == "" {
		exe, e = os.Executable()
		if e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	super := Supervisor{Store: s, Executable: exe, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PerWorkerRSS: 1, TotalRSS: 1}
	go func() { done <- super.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(20 * time.Second):
			t.Error("RSS-limited worker not reaped")
		}
	})
	for deadline := time.Now().Add(20 * time.Second); ; {
		var reason Reason
		var state State
		var pid int
		if e = s.DB.QueryRow(context.Background(), `SELECT runtime_state,reason,worker_pid FROM plugin_v2_instances WHERE id=$1`, first.ID).Scan(&state, &reason, &pid); e != nil {
			t.Fatal(e)
		}
		if state == Backoff && reason == ResourceLimit && pid == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("RSS limit did not converge", state, reason, pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestStderrFatalCategorySurvivesTruncationWithoutPayload(t *testing.T) {
	var out stderrLimit
	if _, err := out.Write([]byte("runtime: failed to create new OS thread /private?token=NEVER_EXPORT")); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte(strings.Repeat("safe-stack\n", 600))); err != nil {
		t.Fatal(err)
	}
	if out.category() != "worker_process_limit" || len(out.tail) > 4096 {
		t.Fatal("classification")
	}
}
