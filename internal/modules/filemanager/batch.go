package filemanager

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"time"
)

type BatchItem struct {
	Ordinal int     `json:"ordinal"`
	Source  string  `json:"source"`
	State   string  `json:"state"`
	Message string  `json:"message"`
	Result  *Result `json:"result,omitempty"`
}
type Batch struct {
	ID        string      `json:"id"`
	Digest    string      `json:"digest"`
	State     string      `json:"state"`
	ExpiresAt time.Time   `json:"expiresAt"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	Items     []BatchItem `json:"items"`
}

func (e Engine) PreviewBatch(ctx context.Context, inputs []Input) (Batch, error) {
	if len(inputs) < 1 || len(inputs) > 20 {
		return Batch{}, ErrLimit
	}
	out := Batch{ID: jobs.UUID(), State: "preview", ExpiresAt: time.Now().UTC().Add(10 * time.Minute), Items: []BatchItem{}}
	sources, targets := map[string]bool{}, map[string]bool{}
	for i, in := range inputs {
		row := BatchItem{Ordinal: i, Source: in.Source, State: "preview"}
		key := in.ConnectionID + ":" + in.Source
		var plan Plan
		var err error
		if sources[key] {
			err = ErrConflict
		} else {
			sources[key] = true
			plan, err = e.Preview(ctx, in)
		}
		if err == nil {
			for _, item := range plan.Items {
				target := plan.ConnectionID + ":" + item.Target
				if targets[target] {
					err = ErrConflict
				}
				targets[target] = true
			}
			if err == nil {
				result := Result{ID: plan.ID, State: "preview", Plan: plan}
				row.Result = &result
				if plan.ExpiresAt.Before(out.ExpiresAt) {
					out.ExpiresAt = plan.ExpiresAt
				}
			}
		}
		if err != nil {
			row.State = "blocked"
			row.Message = err.Error()
		}
		out.Items = append(out.Items, row)
	}
	out.Digest = fingerprint(out.Items)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return out, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO file_batches(id,digest,expires_at)VALUES($1,$2,$3)`, out.ID, out.Digest, out.ExpiresAt); err != nil {
		return out, ErrUnavailable
	}
	for _, row := range out.Items {
		var operation any
		if row.Result != nil {
			operation = row.Result.ID
		}
		if _, err = tx.Exec(ctx, `INSERT INTO file_batch_items(batch_id,ordinal,operation_id,source,state,message)VALUES($1,$2,$3,$4,$5,$6)`, out.ID, row.Ordinal, operation, row.Source, row.State, row.Message); err != nil {
			return out, ErrUnavailable
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return out, ErrUnavailable
	}
	return e.Batch(ctx, out.ID)
}
func (e Engine) Batch(ctx context.Context, id string) (Batch, error) {
	out := Batch{Items: []BatchItem{}}
	if err := e.DB.QueryRow(ctx, `SELECT id::text,digest,state,expires_at,created_at,updated_at FROM file_batches WHERE id=$1`, id).Scan(&out.ID, &out.Digest, &out.State, &out.ExpiresAt, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return out, ErrMissing
	}
	rows, err := e.DB.Query(ctx, `SELECT i.ordinal,i.source,i.state,i.message,coalesce(o.id::text,''),coalesce(o.state,''),coalesce(o.completed_steps,0),coalesce(o.message,''),o.plan FROM file_batch_items i LEFT JOIN file_operations o ON o.id=i.operation_id WHERE i.batch_id=$1 ORDER BY i.ordinal`, id)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var row BatchItem
		var r Result
		var raw []byte
		if err = rows.Scan(&row.Ordinal, &row.Source, &row.State, &row.Message, &r.ID, &r.State, &r.CompletedSteps, &r.Message, &raw); err != nil {
			return out, ErrUnavailable
		}
		if r.ID != "" {
			if json.Unmarshal(raw, &r.Plan) != nil {
				return out, ErrUnavailable
			}
			row.Result = &r
		}
		out.Items = append(out.Items, row)
	}
	return out, rows.Err()
}
func (e Engine) ApproveBatch(ctx context.Context, id, digest string, confirm bool) (Batch, error) {
	if !confirm || digest == "" {
		return Batch{}, ErrInvalid
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return Batch{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var state, stored string
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT state,digest,expires_at FROM file_batches WHERE id=$1 FOR UPDATE`, id).Scan(&state, &stored, &expires); err != nil {
		return Batch{}, ErrMissing
	}
	if stored != digest {
		return Batch{}, ErrChanged
	}
	if state != "preview" {
		return e.Batch(ctx, id)
	}
	if !expires.After(time.Now()) {
		return Batch{}, ErrChanged
	}
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_batch_items WHERE batch_id=$1 AND (state<>'preview' OR operation_id IS NULL))`, id).Scan(&blocked); err != nil {
		return Batch{}, ErrUnavailable
	}
	if blocked {
		return Batch{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE file_batches SET state='running',updated_at=now() WHERE id=$1`, id); err != nil {
		return Batch{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key)VALUES($1,'files.batch','library',jsonb_build_object('batchId',$2::text),$3)`, jobs.UUID(), id, "files.batch:"+id); err != nil {
		return Batch{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','files.batch_approved',jsonb_build_object('batchId',$1::text,'digest',$2::text))`, id, digest); err != nil {
		return Batch{}, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return Batch{}, ErrUnavailable
	}
	return e.Batch(ctx, id)
}
func (e Engine) ProcessBatch(ctx context.Context, job jobs.Job) error {
	var body struct {
		ID string `json:"batchId"`
	}
	if json.Unmarshal(job.Payload, &body) != nil || body.ID == "" {
		return ErrInvalid
	}
	unlock, err := e.lock(ctx, "batch:"+body.ID)
	if err != nil {
		return err
	}
	defer unlock()
	batch, err := e.Batch(ctx, body.ID)
	if err != nil {
		return err
	}
	if batch.State == "succeeded" || batch.State == "partial" {
		return nil
	}
	if batch.State != "running" {
		return jobs.ErrOperatorRequired
	}
	for _, item := range batch.Items {
		if item.State == "succeeded" || item.State == "failed" {
			continue
		}
		if item.State != "preview" || item.Result == nil {
			e.finishBatch(context.WithoutCancel(ctx), body.ID)
			return jobs.ErrOperatorRequired
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err = e.DB.Exec(ctx, `UPDATE file_batch_items SET state='running' WHERE batch_id=$1 AND ordinal=$2 AND state='preview'`, body.ID, item.Ordinal); err != nil {
			return err
		}
		r, workErr := e.Execute(ctx, item.Result.ID, item.Result.Plan.Digest, true)
		state, message := r.State, r.Message
		if workErr != nil {
			observed, readErr := e.Get(context.WithoutCancel(ctx), item.Result.ID)
			state = "needs_reconcile"
			message = workErr.Error()
			if readErr == nil && observed.State == "preview" {
				state = "failed"
			}
		}
		if state == "running" || state == "" {
			state = "needs_reconcile"
		}
		if _, err = e.DB.Exec(context.WithoutCancel(ctx), `UPDATE file_batch_items SET state=$3,message=$4 WHERE batch_id=$1 AND ordinal=$2`, body.ID, item.Ordinal, state, message); err != nil {
			return err
		}
		if state == "needs_reconcile" {
			e.finishBatch(context.WithoutCancel(ctx), body.ID)
			return jobs.ErrOperatorRequired
		}
	}
	return e.finishBatch(ctx, body.ID)
}
func (e Engine) finishBatch(ctx context.Context, id string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT CASE WHEN bool_or(state IN ('running','needs_reconcile')) THEN 'needs_reconcile' WHEN bool_and(state='succeeded') THEN 'succeeded' ELSE 'partial' END FROM file_batch_items WHERE batch_id=$1`, id).Scan(&state); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE file_batches SET state=$2,updated_at=now() WHERE id=$1`, id, state); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]string{"batchId": id, "state": state})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)SELECT $1,$2,$3,$4 WHERE NOT EXISTS(SELECT 1 FROM outbox_events WHERE aggregate_id=$3 AND event_type=$2)`, jobs.UUID(), "files.batch."+state, id, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (e Engine) ReconcileBatch(ctx context.Context, id string) (Batch, error) {
	unlock, err := e.lock(ctx, "batch:"+id)
	if err != nil {
		return Batch{}, err
	}
	defer unlock()
	batch, err := e.Batch(ctx, id)
	if err != nil {
		return batch, err
	}
	if batch.State != "needs_reconcile" && batch.State != "running" {
		return batch, nil
	}
	for _, item := range batch.Items {
		if item.Result == nil || item.State == "succeeded" || item.State == "failed" {
			continue
		}
		if item.State == "preview" {
			continue
		} // No automatic resumption after an unknown result.
		r, err := e.Reconcile(ctx, item.Result.ID)
		if err != nil {
			return batch, err
		}
		state := r.State
		if state == "preview" {
			state = "failed"
			r.Message = "未执行写入；请重新预览剩余项"
		}
		if state == "running" || state == "needs_reconcile" {
			return batch, ErrBusy
		}
		if _, err = e.DB.Exec(ctx, `UPDATE file_batch_items SET state=$3,message=$4 WHERE batch_id=$1 AND ordinal=$2`, id, item.Ordinal, state, r.Message); err != nil {
			return batch, ErrUnavailable
		}
	}
	if err = e.finishBatch(ctx, id); err != nil {
		return batch, err
	}
	return e.Batch(ctx, id)
}

func (e Engine) sharedParent(ctx context.Context, b Backend, p Plan, target string) error {
	var raw []byte
	if err := e.DB.QueryRow(ctx, `SELECT identity FROM file_batch_parents d JOIN file_batch_items i ON i.batch_id=d.batch_id WHERE i.operation_id=$1 AND d.integration_id=$2 AND d.path=$3`, p.ID, p.ConnectionID, target).Scan(&raw); err != nil {
		return ErrConflict
	}
	var expected File
	if json.Unmarshal(raw, &expected) != nil {
		return ErrChanged
	}
	actual, err := b.Find(ctx, target)
	if err != nil || !actual.Directory || expected.ID == "" || expected.ID != actual.ID || actual.Unsupported {
		return ErrChanged
	}
	return nil
}
