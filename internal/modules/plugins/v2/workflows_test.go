package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/pluginstate"
	"strings"
	"testing"
	"time"
)

func workflowFixture(t *testing.T) (Store, Lease, WorkflowRunner, contracts.Workflow) {
	t.Helper()
	s, l := activeFixture(t)
	if _, e := s.DB.Exec(context.Background(), `CREATE TABLE ops_mode (LIKE public.ops_mode INCLUDING ALL); INSERT INTO ops_mode(singleton,paused)VALUES(true,false)`); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	meta := contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{{Kind: "action", ID: "hello", Handler: contracts.HandlerID("action", "hello"), RequiredScopes: []string{"actions.register"}}}}
	if e := s.Publish(ctx, l, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	return s, l, WorkflowRunner{s}, contracts.Workflow{ID: "test-workflow", Steps: []contracts.WorkflowStep{{ID: "one", InstanceID: l.ID, Action: "hello", Input: contracts.ActionContext{Data: map[string]json.RawMessage{}}}}}
}
func TestWorkflowApprovalScopeRevisionCancelAndFactAtomicity(t *testing.T) {
	s, l, r, w := workflowFixture(t)
	ctx := context.Background()
	d, e := r.Save(ctx, "admin", w, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Start(ctx, "admin", w.ID, "unapproved-key"); e == nil {
		t.Fatal("unapproved")
	}
	if _, e = r.Approve(ctx, "admin", w.ID, "stale"); e == nil {
		t.Fatal("fingerprint")
	}
	d, e = r.Approve(ctx, "admin", w.ID, d.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	reject := dbutil.WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { return ErrConflict })
	if _, e = r.Start(reject, "admin", w.ID, "rollback-key"); e == nil {
		t.Fatal("rollback")
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='workflow.started'`).Scan(&n); e != nil || n != 0 {
		t.Fatal("rolled back fact", n, e)
	}
	run, e := r.Start(ctx, "admin", w.ID, "approved-key")
	if e != nil {
		t.Fatal(e)
	}
	twice, e := r.Start(ctx, "admin", w.ID, "approved-key")
	if e != nil || twice.ID != run.ID {
		t.Fatal("idempotency", twice, e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='workflow.started'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("commit fact", n, e)
	}
	if _, e = r.Cancel(ctx, "admin", w.ID, run.ID); e != nil {
		t.Fatal(e)
	}
	got, e := r.Status(ctx, w.ID, run.ID)
	if e != nil || got.State != "canceled" {
		t.Fatal(got, e)
	}
	d, e = r.Save(ctx, "admin", w, d.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Start(ctx, "admin", w.ID, "edited-key"); e == nil {
		t.Fatal("edit retained approval")
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='actions.register'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Approve(ctx, "admin", w.ID, d.Fingerprint); e == nil {
		t.Fatal("action missing scope")
	}
}
func TestWorkflowUnknownNeverReplayedAndStatusFence(t *testing.T) {
	s, l, r, w := workflowFixture(t)
	ctx := context.Background()
	d, e := r.Save(ctx, "admin", w, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Approve(ctx, "admin", w.ID, d.Fingerprint); e != nil {
		t.Fatal(e)
	}
	run, e := r.Start(ctx, "admin", w.ID, "unknown-test")
	if e != nil {
		t.Fatal(e)
	}
	repo := jobs.Repository{DB: s.DB}
	j, e := repo.Claim(ctx, "fixture", time.Minute)
	if e != nil || j == nil {
		t.Fatal(j, e)
	}
	// Simulate an interrupted started step: jobs max_attempts=1 and runner refuses replay.
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var current workflowExecution
	key := "plugins.workflow.run." + run.ID
	if e = pluginstate.Lock(ctx, tx, key, &current); e != nil {
		t.Fatal(e)
	}
	current.State = "running"
	current.Steps["one"] = "started"
	if e = pluginstate.Put(ctx, tx, key, current); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = r.Process(ctx, *j); !errors.Is(e, jobs.ErrOperatorRequired) {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	got, e := r.Status(ctx, w.ID, run.ID)
	if e != nil || got.State != "unknown" {
		t.Fatal(got, e)
	}
	next, e := repo.Claim(ctx, "other", time.Minute)
	if e != nil || next != nil {
		t.Fatal("replayed exhausted job", next, e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='actions.register'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Start(ctx, "admin", w.ID, "revoked-test"); e == nil {
		t.Fatal("revoked")
	}
}
func TestWorkflowHostExactSelectorsAndLiveRevoke(t *testing.T) {
	s, l, r, w := workflowFixture(t)
	ctx := context.Background()
	d, e := r.Save(ctx, "admin", w, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Approve(ctx, "admin", w.ID, d.Fingerprint); e != nil {
		t.Fatal(e)
	}
	p := contracts.Permission{Scope: "workflows.run", Resources: map[string][]string{"workflowIds": {w.ID}}}
	m := contracts.Manifest{Permissions: []contracts.Permission{p}}
	if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,'workflows.run',$2)`, l.ID, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	var run contracts.WorkflowRun
	for _, method := range []string{"workflows.run", "workflows.status", "workflows.cancel"} {
		q := contracts.WorkflowRequest{WorkflowID: w.ID, RunID: run.ID}
		raw, _ := json.Marshal(q)
		call := contracts.HostCall{Method: method, Args: raw, OperationKey: method + "-key"}
		if _, e = s.hostDeclared(ctx, l, call, contracts.Manifest{}); e == nil {
			t.Fatal("undeclared", method)
		}
		out, e := s.hostDeclared(ctx, l, call, m)
		if e != nil {
			t.Fatal(method, e)
		}
		b, _ := json.Marshal(out)
		if json.Unmarshal(b, &run) != nil {
			t.Fatal("run dto")
		}
		if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='workflows.run'`, l.ID); e != nil {
			t.Fatal(e)
		}
		if _, e = s.hostDeclared(ctx, l, call, m); e == nil {
			t.Fatal("revoked", method)
		}
		if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,'workflows.run',$2)`, l.ID, l.Fingerprint); e != nil {
			t.Fatal(e)
		}
	}
	q, _ := json.Marshal(contracts.WorkflowRequest{WorkflowID: "unapproved"})
	_, e = s.hostDeclared(ctx, l, contracts.HostCall{Method: "workflows.run", Args: q, OperationKey: "wrong-selector"}, m)
	if e == nil {
		t.Fatal("selector escaped")
	}
}

func TestWorkflowPolicyNarrowingInvalidTimeoutAndScope(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	d := contracts.Descriptor{Kind: "chain", ID: "workflow.execution", Handler: contracts.HandlerID("chain", "workflow.execution"), RequiredScopes: []string{"events.chain.workflow.execution"}}
	if e := s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	r := WorkflowRunner{s}
	q := contracts.WorkflowBeforeStep{WorkflowID: "safe", StepID: "one", Allow: true}
	for _, tc := range []struct {
		raw    string
		reject bool
	}{{`{"allow":false}`, true}, {`{"allow":true,"grantAdmin":true}`, false}, {`{}`, false}} {
		done := policyReply(s, json.RawMessage(tc.raw), false)
		prepared, e := r.BeforeStep(ctx, q)
		if tc.reject != (e != nil) {
			t.Fatal(tc, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if !tc.reject {
			if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
				t.Fatal(e)
			}
		}
	}
	start := time.Now()
	if _, e := r.BeforeStep(ctx, q); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("200ms policy unbounded")
	}
	if _, e := s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='events.chain.workflow.execution'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Descriptor(ctx, l.ID, "chain", "workflow.execution"); e == nil {
		t.Fatal("revoked hook")
	}
	if _, e := r.BeforeStep(ctx, q); e != nil {
		t.Fatal("fallback", e)
	}
}

func TestWorkflowStartedDeliveryCommitRollbackRevokeAndSecretProjection(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	typ := "workflow.started"
	d := contracts.Descriptor{Kind: "event", ID: typ, Handler: contracts.HandlerID("event", typ), RequiredScopes: []string{"events.subscribe." + typ}}
	if e := s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	emit := func(commit bool, version string) {
		t.Helper()
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		fact := map[string]any{"runId": "fact", "workflowId": "test", "revision": 1, "secret": "NEVER_BRIDGE_SECRET"}
		if e = outbox.EmitVersionTx(ctx, tx, typ, "fact", version, fact); e != nil {
			t.Fatal(e)
		}
		if commit {
			e = tx.Commit(ctx)
		} else {
			e = tx.Rollback(ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
	}
	emit(false, "v1")
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	emit(true, "v1")
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	var raw string
	if e := s.DB.QueryRow(ctx, `SELECT payload::text FROM plugin_v2_event_journal WHERE event_type=$1`, typ).Scan(&raw); e != nil || strings.Contains(raw, "SECRET") {
		t.Fatal("projection", raw, e)
	}
	if _, e := s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, "events.subscribe."+typ); e != nil {
		t.Fatal(e)
	}
	emit(true, "v2")
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("revoked event", n, e)
	}
}
