package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/outbox"
	"strings"
	"testing"
	"time"
)

func snapshotFixture(t *testing.T) (Store, Lease, Snapshot) {
	t.Helper()
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	metadata := contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{}}
	for _, a := range defs[0].Manifest.Extensions.API {
		required := append([]string{"api.register"}, a.ExecutionScopes...)
		if a.Auth == "public" {
			required = append(required, "api.public")
		}
		metadata.Descriptors = append(metadata.Descriptors, contracts.Descriptor{Kind: "api", ID: a.ID, Path: a.Path, Methods: a.Methods, Auth: a.Auth, Handler: contracts.HandlerID("api", a.ID), RequiredScopes: required})
	}
	metadata.Descriptors = append(metadata.Descriptors, contracts.Descriptor{Kind: "service", ID: "tick", Trigger: "interval", IntervalMS: 1000, Handler: contracts.HandlerID("service", "tick"), RequiredScopes: []string{"services.register"}})
	metadata.Descriptors = append(metadata.Descriptors, contracts.Descriptor{Kind: "event", ID: "library.organized", Handler: contracts.HandlerID("event", "library.organized"), RequiredScopes: []string{"events.subscribe.library.organized"}})
	if e := s.Publish(ctx, l, metadata); e != nil {
		t.Fatal(e)
	}
	if e := s.Publish(ctx, l, metadata); e != nil {
		t.Fatal("duplicate snapshot", e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	snap, e := s.Snapshot(ctx, l.ID)
	if e != nil {
		t.Fatal(e)
	}
	return s, l, snap
}
func TestExtensionSnapshotsScopesConflictAndImmutableHash(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_snapshots`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	changed := snap.Metadata
	changed.Descriptors = append([]contracts.Descriptor{}, changed.Descriptors...)
	changed.Descriptors[0].Description = "changed"
	if e := s.Publish(ctx, l, changed); !errors.Is(e, ErrConflict) {
		t.Fatal("snapshot was mutable", e)
	}
	missing := snap.Metadata
	missing.Descriptors = append([]contracts.Descriptor{}, missing.Descriptors...)
	missing.Descriptors[0].RequiredScopes = []string{"system.exec"}
	if e := s.Publish(ctx, l, missing); e == nil {
		t.Fatal("scope expansion accepted")
	}
	if _, e := s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='api.register'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Descriptor(ctx, l.ID, "api", "/admin"); e == nil {
		t.Fatal("cached grants")
	}
}
func TestMailboxIdempotencyLeaseUnknownAndRevocation(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	_, d, e := s.Descriptor(ctx, l.ID, "api", "admin")
	if e != nil {
		t.Fatal(e)
	}
	key := "owned-idempotency"
	run, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), key, "", nil, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), key, "", nil, time.Now().Add(time.Minute))
	if e != nil || run != again {
		t.Fatal(run, again, e)
	}
	if _, e = s.Submit(ctx, snap, d, json.RawMessage(`{"changed":true}`), key, "", nil, time.Now().Add(time.Minute)); !errors.Is(e, ErrConflict) {
		t.Fatal("idempotency mismatch", e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='running',lease_token=$2,lease_until=now()-interval '1 second' WHERE id=$1`, run, jobs.UUID()); e != nil {
		t.Fatal(e)
	}
	if e = s.MaintainExtensions(ctx); e != nil {
		t.Fatal(e)
	}
	var state RunState
	if e = s.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE id=$1`, run).Scan(&state); e != nil || state != RunUnknown {
		t.Fatal(state, e)
	}
	pending, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), "pending-revoked", "", nil, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state='disabled',grant_revision=grant_revision+1 WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.MaintainExtensions(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE id=$1`, pending).Scan(&state); e != nil || state != RunCanceled {
		t.Fatal(state, e)
	}
	if _, e = s.Submit(ctx, snap, d, json.RawMessage(`{}`), "late-call", "", nil, time.Now().Add(time.Minute)); e == nil {
		t.Fatal("old generation accepted")
	}
}
func TestAPIKeysAreScopedExpiringRevocableAndRateBounded(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	key, e := s.CreateAPIKey(ctx, "admin", l.ID, "key", []string{"apikey:GET", "bearer:GET"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CheckAPIKey(ctx, l.ID, "apikey", "GET", key.Token, snap); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct{ route, method, token string }{{"admin", "GET", key.Token}, {"apikey", "POST", key.Token}, {"apikey", "GET", strings.Repeat("x", 74)}} {
		if s.CheckAPIKey(ctx, l.ID, v.route, v.method, v.token, snap) == nil {
			t.Fatal("bad credential accepted")
		}
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_api_keys SET expires_at=now()-interval '1 second' WHERE id=$1`, key.ID); e != nil {
		t.Fatal(e)
	}
	if s.CheckAPIKey(ctx, l.ID, "apikey", "GET", key.Token, snap) == nil {
		t.Fatal("expired key accepted")
	}
	if e = s.RevokeAPIKey(ctx, "admin", l.ID, key.ID); e != nil {
		t.Fatal(e)
	}
	for range 60 {
		if e = s.LimitAPI(ctx, l.ID); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.LimitAPI(ctx, l.ID); !errors.Is(e, ErrRateLimited) {
		t.Fatal(e)
	}
}
func TestServiceSchedulesCoalesceAndDoNotAccumulate(t *testing.T) {
	s, l, _ := snapshotFixture(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, `UPDATE plugin_v2_schedules SET next_at=now()-interval '1 day' WHERE instance_id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	for range 3 {
		if e := s.Schedule(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='plugin.v2.service'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("accumulated jobs", n, e)
	}
	var j jobs.Job
	if e := s.DB.QueryRow(ctx, `SELECT id::text,payload FROM jobs WHERE kind='plugin.v2.service'`).Scan(&j.ID, &j.Payload); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e := s.ProcessService(ctx, j); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='service'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("replayed wake duplicated service", n, e)
	}
}
func TestOutboxRollbackLateFactsCursorReplayAndUnknown(t *testing.T) {
	s, l, _ := snapshotFixture(t)
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = outbox.EmitTx(ctx, tx, "files.succeeded", jobs.UUID(), map[string]any{"message": "private path omitted"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_event_journal`).Scan(&n); e != nil || n != 0 {
		t.Fatal("rolled back fact emitted", n, e)
	}
	delayed, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = outbox.EmitTx(ctx, delayed, "files.succeeded", jobs.UUID(), map[string]any{"message": "secret"}); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = delayed.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='unknown',reason='worker_lost' WHERE kind='event'`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Replay(ctx, "admin", l.ID, 0, 100, false); e != nil {
		t.Fatal(e)
	}
	var state RunState
	if e = s.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE kind='event'`).Scan(&state); e != nil || state != RunUnknown {
		t.Fatal("unknown automatically retried", state, e)
	}
	if _, e = s.Replay(ctx, "admin", l.ID, 0, 100, true); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE kind='event'`).Scan(&state); e != nil || state != RunPending {
		t.Fatal(state, e)
	}
}
func TestServiceClockKindsSettingsAndMisfire(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	for _, v := range []contracts.Descriptor{{Trigger: "cron", Cron: "*/5 * * * *"}, {Trigger: "interval", IntervalMS: 1000}, {Trigger: "date", At: now.Add(time.Hour).Format(time.RFC3339)}, {Trigger: "once"}} {
		at, on, e := nextService(v, json.RawMessage(`{}`), now)
		if e != nil || !on || at.Before(now) {
			t.Fatal(v, at, on, e)
		}
	}
	d := contracts.Descriptor{Trigger: "interval", IntervalMS: 1000, ScheduleSetting: "timer"}
	if _, on, e := nextService(d, json.RawMessage(`{"timer":false}`), now); e != nil || on {
		t.Fatal(on, e)
	}
	at, on, e := nextService(d, json.RawMessage(`{"timer":3}`), now)
	if e != nil || !on || !at.Equal(now.Add(3*time.Second)) {
		t.Fatal(at, e)
	}
}

func TestLateCallCompletionFenceAndManualServiceCoalescing(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	_, d, e := s.Descriptor(ctx, l.ID, "api", "admin")
	if e != nil {
		t.Fatal(e)
	}
	run, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), "late-write-fence", "", nil, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	token := jobs.UUID()
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='running',lease_token=$2,lease_until=now()+interval '30 seconds' WHERE id=$1`, run, token); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET grant_revision=grant_revision+1 WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.finishCall(ctx, run, token, RunSucceeded, RunNoReason, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("late write escaped fence", e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET grant_revision=grant_revision-1 WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	a, e := s.RunService(ctx, "admin", l.ID, "tick", "manual-first")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.RunService(ctx, "admin", l.ID, "tick", "manual-second")
	if e != nil || a != b {
		t.Fatal("manual backlog", a, b, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE jobs SET state='succeeded' WHERE id=$1`, a); e != nil {
		t.Fatal(e)
	}
	replay, e := s.RunService(ctx, "admin", l.ID, "tick", "manual-second")
	if e != nil || replay != b {
		t.Fatal("manual operation replayed", e)
	}
}

func TestGlobalCommandConflictAndPriorityOrder(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	metadata := contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{{Kind: "command", ID: "hello", Cmd: "hello", Handler: contracts.HandlerID("command", "hello"), RequiredScopes: []string{"commands.register"}}, {Kind: "service", ID: "tick", Trigger: "once", Priority: 50, Handler: contracts.HandlerID("service", "tick"), RequiredScopes: []string{"services.register"}}}}
	if e := s.Publish(ctx, l, metadata); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	snap, e := s.Snapshot(ctx, l.ID)
	if e != nil || snap.Metadata.Descriptors[0].Priority != 50 {
		t.Fatal("priority not applied", e)
	}
	defs, _ := Catalog()
	d := defs[0]
	i, e := s.Apply(ctx, "admin", Request{Kind: Install, OperationKey: "install-second", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "admin", Request{Kind: Enable, OperationKey: "enable-second", InstanceID: i.ID, Fingerprint: d.Fingerprint, ExpectedRevision: i.Revision, Confirm: true}); e != nil {
		t.Fatal(e)
	}
	second, e := s.Claim(ctx, jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Publish(ctx, second, metadata); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate global command accepted", e)
	}
}

func TestAPIScopeCeilingCannotBeExpandedByWorkerParameters(t *testing.T) {
	gate := &hostGate{}
	gate.enter([]string{"api.register", "api.public", "plugin.data.read"})
	call := (Store{}).scopedHost(Lease{}, gate)
	_, rpc := call(context.Background(), json.RawMessage(`{"method":"plugin.data.put","args":{"key":"x","value":true},"expectedRevision":0}`))
	if rpc == nil || rpc.Code != contracts.PermissionDenied {
		t.Fatal("API scope ceiling was not enforced", rpc)
	}
}
