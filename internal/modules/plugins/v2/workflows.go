package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/pluginstate"
	"media-tool/internal/platform/requestbudget"
	"time"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
)

type WorkflowRunner struct{ Store Store }
type workflowGrant struct {
	Instance    string `json:"instance"`
	Fingerprint string `json:"fingerprint"`
	Revision    int64  `json:"revision"`
	Descriptor  string `json:"descriptor"`
}
type WorkflowDefinition struct {
	DAG         contracts.Workflow       `json:"dag"`
	Revision    int64                    `json:"revision"`
	Fingerprint string                   `json:"fingerprint"`
	Approved    string                   `json:"approved"`
	Grants      map[string]workflowGrant `json:"grants"`
}
type workflowExecution struct {
	contracts.WorkflowRun
	Definition WorkflowDefinition `json:"definition"`
	Expires    time.Time          `json:"expires"`
	Steps      map[string]string  `json:"steps"`
}

func workflowHash(v any) string { b, _ := json.Marshal(v); return digest(b) }
func (r WorkflowRunner) load(ctx context.Context, key string, out any) error {
	var b []byte
	e := r.Store.DB.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, key).Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func workflowAudit(ctx context.Context, tx pgx.Tx, actor, action, id string) error {
	raw, _ := json.Marshal(map[string]string{"id": id})
	_, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3)`, actor, action, raw)
	return e
}

// Save always clears approval, even if the new graph happens to be equivalent.
func (r WorkflowRunner) Save(ctx context.Context, actor string, w contracts.Workflow, revision int64) (WorkflowDefinition, error) {
	if _, e := contracts.WorkflowOrder(w); e != nil {
		return WorkflowDefinition{}, ErrInvalid
	}
	tx, e := r.Store.DB.Begin(ctx)
	if e != nil {
		return WorkflowDefinition{}, e
	}
	defer rollback(ctx, tx)
	v := WorkflowDefinition{}
	key := "plugins.workflow." + w.ID
	if e = pluginstate.Lock(ctx, tx, key, &v); e != nil {
		return v, e
	}
	if v.Revision != revision {
		return v, ErrConflict
	}
	v = WorkflowDefinition{DAG: w, Revision: revision + 1, Fingerprint: workflowHash(w), Grants: map[string]workflowGrant{}}
	if e = pluginstate.Put(ctx, tx, key, v); e == nil {
		e = workflowAudit(ctx, tx, actor, "plugins.workflow.save", w.ID)
	}
	if e == nil {
		e = dbutil.Commit(ctx, tx)
	}
	return v, e
}
func (r WorkflowRunner) approveSnapshot(ctx context.Context, v *WorkflowDefinition) error {
	v.Grants = map[string]workflowGrant{}
	for _, n := range v.DAG.Steps {
		snap, d, e := r.Store.Descriptor(ctx, n.InstanceID, "action", n.Action)
		if e != nil {
			return e
		}
		var fp string
		if e = r.Store.DB.QueryRow(ctx, `SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1`, n.InstanceID).Scan(&fp); e != nil {
			return e
		}
		v.Grants[n.ID] = workflowGrant{n.InstanceID, fp, snap.GrantRevision, workflowHash(d)}
		ctx = dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error { return r.Store.fencePolicy(c, tx, snap, d) })
	}
	return dbutil.CheckPrepared(ctx, r.Store.DB)
}
func (r WorkflowRunner) Approve(ctx context.Context, actor, id, fingerprint string) (WorkflowDefinition, error) {
	var v WorkflowDefinition
	if e := r.load(ctx, "plugins.workflow."+id, &v); e != nil {
		return v, e
	}
	if fingerprint == "" || v.Fingerprint != fingerprint {
		return v, ErrConflict
	}
	if e := r.approveSnapshot(ctx, &v); e != nil {
		return v, e
	}
	tx, e := r.Store.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer rollback(ctx, tx)
	var current WorkflowDefinition
	if e = pluginstate.Lock(ctx, tx, "plugins.workflow."+id, &current); e != nil {
		return v, e
	}
	if current.Revision != v.Revision {
		return v, ErrConflict
	}
	for _, n := range v.DAG.Steps {
		if e = r.fenceStep(ctx, tx, v, n); e != nil {
			return v, e
		}
	}
	v.Approved = fingerprint
	if e = pluginstate.Put(ctx, tx, "plugins.workflow."+id, v); e == nil {
		e = workflowAudit(ctx, tx, actor, "plugins.workflow.approve", id)
	}
	if e == nil {
		e = dbutil.Commit(ctx, tx)
	}
	return v, e
}
func (r WorkflowRunner) fenceStep(ctx context.Context, tx pgx.Tx, v WorkflowDefinition, n contracts.WorkflowStep) error {
	g, ok := v.Grants[n.ID]
	if !ok || g.Instance != n.InstanceID {
		return ErrConflict
	}
	// Fetching metadata performs no worker RPC. Final SQL fence intersects every action scope.
	snap, d, e := r.Store.Descriptor(ctx, n.InstanceID, "action", n.Action)
	if e != nil {
		return e
	}
	if snap.GrantRevision != g.Revision || workflowHash(d) != g.Descriptor {
		return ErrConflict
	}
	var fp string
	if e = tx.QueryRow(ctx, `SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1`, n.InstanceID).Scan(&fp); e != nil {
		return e
	}
	if fp != g.Fingerprint {
		return ErrConflict
	}
	return r.Store.fencePolicy(ctx, tx, snap, d)
}
func (r WorkflowRunner) Start(ctx context.Context, actor, id, key string) (contracts.WorkflowRun, error) {
	if len(key) < 8 || len(key) > 100 {
		return contracts.WorkflowRun{}, ErrInvalid
	}
	tx, e := r.Store.DB.Begin(ctx)
	if e != nil {
		return contracts.WorkflowRun{}, e
	}
	defer rollback(ctx, tx)
	var v WorkflowDefinition
	if e = pluginstate.Lock(ctx, tx, "plugins.workflow."+id, &v); e != nil {
		return contracts.WorkflowRun{}, e
	}
	if v.Approved == "" || v.Approved != v.Fingerprint || v.Fingerprint != workflowHash(v.DAG) {
		return contracts.WorkflowRun{}, ErrConflict
	}
	if _, e = contracts.WorkflowOrder(v.DAG); e != nil {
		return contracts.WorkflowRun{}, ErrInvalid
	}
	runID := workflowHash([]string{actor, id, key})
	stateKey := "plugins.workflow.run." + runID
	var run workflowExecution
	if e = pluginstate.Lock(ctx, tx, stateKey, &run); e != nil {
		return run.WorkflowRun, e
	}
	if run.ID != "" {
		if run.Definition.Revision != v.Revision {
			return run.WorkflowRun, ErrConflict
		}
		return run.WorkflowRun, tx.Commit(ctx)
	}
	for _, n := range v.DAG.Steps {
		if e = r.fenceStep(ctx, tx, v, n); e != nil {
			return run.WorkflowRun, e
		}
	}
	run = workflowExecution{WorkflowRun: contracts.WorkflowRun{ID: runID, WorkflowID: id, State: "pending"}, Definition: v, Expires: time.Now().Add(30 * time.Minute), Steps: map[string]string{}}
	payload, _ := json.Marshal(map[string]string{"runId": runID})
	_, e = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,max_attempts)VALUES($1,'plugin.v2.workflow',$2,1)`, jobs.UUID(), payload)
	if e == nil {
		e = pluginstate.Put(ctx, tx, stateKey, run)
	}
	if e == nil {
		e = outbox.EmitVersionTx(ctx, tx, string(contracts.EventWorkflowExecute), runID, "v1", contracts.WorkflowStarted{RunID: runID, WorkflowID: id, Revision: v.Revision})
	}
	if e == nil {
		e = workflowAudit(ctx, tx, actor, "plugins.workflow.run", id)
	}
	if e == nil {
		e = dbutil.Commit(ctx, tx)
	}
	return run.WorkflowRun, e
}
func (r WorkflowRunner) Status(ctx context.Context, id, runID string) (contracts.WorkflowRun, error) {
	var v workflowExecution
	e := r.load(ctx, "plugins.workflow.run."+runID, &v)
	if e != nil {
		return v.WorkflowRun, e
	}
	if v.WorkflowID != id {
		return contracts.WorkflowRun{}, ErrNotFound
	}
	// Interrupted execution is never replayed. Surface unknown once the job lease expires.
	if v.State == "running" {
		var live bool
		e = r.Store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE kind='plugin.v2.workflow' AND payload->>'runId'=$1 AND state='running' AND lease_until>clock_timestamp())`, runID).Scan(&live)
		if e != nil {
			return v.WorkflowRun, e
		}
		if !live {
			v.State = "unknown"
		}
	}
	return v.WorkflowRun, nil
}
func (r WorkflowRunner) Cancel(ctx context.Context, actor, id, runID string) (contracts.WorkflowRun, error) {
	tx, e := r.Store.DB.Begin(ctx)
	if e != nil {
		return contracts.WorkflowRun{}, e
	}
	defer rollback(ctx, tx)
	var v workflowExecution
	key := "plugins.workflow.run." + runID
	if e = pluginstate.Lock(ctx, tx, key, &v); e != nil {
		return v.WorkflowRun, e
	}
	if v.ID == "" || v.WorkflowID != id {
		return v.WorkflowRun, ErrNotFound
	}
	if v.State == "pending" || v.State == "running" {
		v.State = "canceled"
		if e = pluginstate.Put(ctx, tx, key, v); e != nil {
			return v.WorkflowRun, e
		}
		_, e = tx.Exec(ctx, `UPDATE plugin_v2_runs SET state='canceled',reason='revoked',finished_at=clock_timestamp()WHERE dedupe_key LIKE $1 AND state IN('pending','running')`, "workflow:"+runID+":%")
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE jobs SET state='cancelled',lease_token=NULL,lease_until=NULL,locked_by=NULL WHERE kind='plugin.v2.workflow' AND payload->>'runId'=$1 AND state IN('pending','running','retry')`, runID)
		}
	}
	if e == nil {
		e = workflowAudit(ctx, tx, actor, "plugins.workflow.cancel", id)
	}
	if e == nil {
		e = dbutil.Commit(ctx, tx)
	}
	return v.WorkflowRun, e
}
func (r WorkflowRunner) BeforeStep(ctx context.Context, q contracts.WorkflowBeforeStep) (context.Context, error) {
	allow := true
	ctx, e := r.Store.acquisitionChain(ctx, contracts.ChainWorkflowExecution, &q, func(_ context.Context, b json.RawMessage) bool {
		var v struct {
			Allow *bool `json:"allow"`
		}
		if contracts.Decode(b, &v) != nil || v.Allow == nil {
			return false
		}
		allow = allow && *v.Allow
		return true
	})
	if e == nil && !allow {
		e = ErrResourceRejected
	}
	return ctx, e
}
func workflowLeaseContext(ctx context.Context, j jobs.Job) context.Context {
	return dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE jobs SET updated_at=updated_at WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>clock_timestamp()`, j.ID, j.LeaseToken)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return jobs.ErrLeaseLost
		}
		return nil
	})
}
func (r WorkflowRunner) Process(ctx context.Context, j jobs.Job) error {
	ctx = workflowLeaseContext(ctx, j)
	var q struct {
		RunID string `json:"runId"`
	}
	if contracts.Decode(j.Payload, &q) != nil {
		return ErrInvalid
	}
	var v workflowExecution
	if e := r.load(ctx, "plugins.workflow.run."+q.RunID, &v); e != nil {
		return e
	}
	if v.State != "pending" {
		return jobs.ErrOperatorRequired
	} // running/unknown/failed never replay.
	ctx, stop := context.WithDeadline(ctx, v.Expires)
	defer stop()
	order, e := contracts.WorkflowOrder(v.Definition.DAG)
	if e != nil {
		return e
	}
	for _, n := range order {
		if e = r.step(ctx, j, &v, n); e != nil {
			return e
		}
	}
	return nil
}
func (r WorkflowRunner) step(ctx context.Context, j jobs.Job, v *workflowExecution, n contracts.WorkflowStep) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	prepared, e := r.BeforeStep(ctx, contracts.WorkflowBeforeStep{WorkflowID: v.WorkflowID, StepID: n.ID, Allow: true})
	if e != nil {
		return r.finishWorkflow(ctx, v, "failed")
	}
	tx, e := r.Store.DB.Begin(prepared)
	if e != nil {
		return e
	}
	defer rollback(prepared, tx)
	var current workflowExecution
	key := "plugins.workflow.run." + v.ID
	if e = pluginstate.Lock(prepared, tx, key, &current); e != nil {
		return e
	}
	if current.State == "canceled" {
		return jobs.ErrOperatorRequired
	}
	if current.Steps[n.ID] != "" {
		return jobs.ErrOperatorRequired
	}
	var live bool
	if e = tx.QueryRow(prepared, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>clock_timestamp())`, j.ID, j.LeaseToken).Scan(&live); e != nil {
		return e
	}
	if !live {
		return jobs.ErrLeaseLost
	}
	var def WorkflowDefinition
	if e = pluginstate.Lock(prepared, tx, "plugins.workflow."+v.WorkflowID, &def); e != nil {
		return e
	}
	if def.Approved != v.Definition.Approved || def.Revision != v.Definition.Revision {
		return r.failAfterRollback(ctx, tx, v)
	}
	if e = r.fenceStep(prepared, tx, v.Definition, n); e != nil {
		return r.failAfterRollback(ctx, tx, v)
	}
	snap, d, e := r.Store.Descriptor(prepared, n.InstanceID, "action", n.Action)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(n.Input)
	run, e := r.Store.submitTx(prepared, tx, snap, d, raw, "workflow:"+v.ID+":"+n.ID, "", nil, minTime(v.Expires, time.Now().Add(30*time.Second)))
	if e != nil {
		return e
	}
	current.State = "running"
	current.Step = n.ID
	current.Steps[n.ID] = "started"
	if e = pluginstate.Put(prepared, tx, key, current); e != nil {
		return e
	}
	if e = dbutil.Commit(prepared, tx); e != nil {
		return e
	}
	// Wait only after releasing every write lock. A lost result is unknown, never retried.
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	state := "unknown"
	for state == "unknown" {
		var s string
		e = r.Store.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE id=$1`, run).Scan(&s)
		if e != nil {
			break
		}
		if s != "pending" && s != "running" {
			state = s
			if state == "succeeded" {
				var ciphertext []byte
				if e = r.Store.DB.QueryRow(ctx, `SELECT result_ciphertext FROM plugin_v2_runs WHERE id=$1`, run).Scan(&ciphertext); e != nil {
					state = "unknown"
				} else {
					raw, openErr := r.Store.open("call-result", run, ciphertext)
					var result contracts.ActionResult
					if openErr != nil || contracts.Decode(raw, &result) != nil || result.Context.Data == nil || len(raw) > 16<<10 {
						state = "unknown"
					} else if !result.Continue {
						state = "failed"
					}
				}
			}
			break
		}
		select {
		case <-ctx.Done():
			return r.finishWorkflow(context.WithoutCancel(ctx), v, "unknown")
		case <-tick.C:
		}
	}
	*v = current
	if state != "succeeded" {
		return r.finishWorkflow(context.WithoutCancel(ctx), v, state)
	}
	tx, e = r.Store.DB.Begin(prepared)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	if e = pluginstate.Lock(ctx, tx, key, &current); e != nil {
		return e
	}
	if current.State == "canceled" {
		return jobs.ErrOperatorRequired
	}
	if e = r.fenceStep(ctx, tx, v.Definition, n); e != nil {
		return r.failAfterRollback(ctx, tx, v)
	}
	if e = r.Store.fencePolicy(ctx, tx, snap, d); e != nil {
		return r.failAfterRollback(ctx, tx, v)
	}
	if e = pluginstate.Lock(ctx, tx, "plugins.workflow."+v.WorkflowID, &def); e != nil {
		return e
	}
	if def.Revision != v.Definition.Revision || def.Approved != v.Definition.Approved {
		return r.failAfterRollback(ctx, tx, v)
	}
	current.Steps[n.ID] = "succeeded"
	current.State = "running"
	if len(current.Steps) == len(current.Definition.DAG.Steps) {
		current.State = "succeeded"
	}
	if e = pluginstate.Put(ctx, tx, key, current); e != nil {
		return e
	}
	if e = dbutil.Commit(prepared, tx); e != nil {
		return e
	}
	*v = current
	return nil
}
func minTime(a, b time.Time) time.Time { return requestbudget.Earlier(a, b) }
func (r WorkflowRunner) failAfterRollback(ctx context.Context, tx pgx.Tx, v *workflowExecution) error {
	rollback(ctx, tx)
	return r.finishWorkflow(context.WithoutCancel(ctx), v, "failed")
}
func (r WorkflowRunner) finishWorkflow(ctx context.Context, v *workflowExecution, state string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, e := r.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	var current workflowExecution
	key := "plugins.workflow.run." + v.ID
	if e = pluginstate.Lock(ctx, tx, key, &current); e != nil {
		return e
	}
	if current.State != "canceled" {
		current.State = state
		if e = pluginstate.Put(ctx, tx, key, current); e != nil {
			return e
		}
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return e
	}
	return fmt.Errorf("%w: workflow %s", jobs.ErrOperatorRequired, state)
}
