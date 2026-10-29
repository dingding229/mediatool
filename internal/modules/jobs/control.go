package jobs

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

var ErrControlConflict = errors.New("任务状态已变化，请重新读取核对")

type ControlResult struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	Revision        int64  `json:"revision"`
	Queued          bool   `json:"queued"`
	StopPending     bool   `json:"stopPending"`
	ExternalEffects string `json:"externalEffects"`
	Reason          string `json:"reason"`
}
type Verification struct {
	Complete  bool
	NextJobID string
	Reason    string
}
type VerifyJob func(context.Context, Job) (Verification, error)

func (r Repository) ControlStatus(ctx context.Context, id string) (ControlResult, error) {
	var v ControlResult
	var requested, ack bool
	err := r.DB.QueryRow(ctx, `SELECT id::text,state,control_revision,cancel_requested_at IS NOT NULL,cancel_acknowledged_at IS NOT NULL,coalesce(last_error,'') FROM jobs WHERE id=$1`, id).Scan(&v.ID, &v.State, &v.Revision, &requested, &ack, &v.Reason)
	v.Queued = v.State == "pending" || v.State == "retry"
	v.StopPending = requested && !ack && v.State == "running"
	v.ExternalEffects = "取消不会撤销已发生的外部操作；未知结果必须核对"
	if v.StopPending {
		v.Reason = "已请求协作停止，尚未确认退出"
	} else if v.Reason == "" {
		v.Reason = "请按任务当前状态核对"
	}
	return v, err
}

// Cancel never deletes files or sends a destructive provider cancellation. It
// stops local scheduling and cooperatively cancels the current worker context.
func (r Repository) Cancel(ctx context.Context, id string, revision int64) (ControlResult, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ControlResult{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	var state string
	var rev int64
	var requested bool
	if err = tx.QueryRow(ctx, `SELECT state,control_revision,cancel_requested_at IS NOT NULL FROM jobs WHERE id=$1 FOR UPDATE`, id).Scan(&state, &rev, &requested); err != nil {
		return ControlResult{}, err
	}
	if requested || state == "cancelled" || state == "succeeded" {
		_ = tx.Rollback(ctx)
		return r.ControlStatus(ctx, id)
	}
	if rev != revision || state == "checking" {
		return ControlResult{}, ErrControlConflict
	}
	next := "cancelled"
	if state == "unknown" {
		next = "unknown"
	}
	if state == "running" {
		next = "running"
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET state=$2,cancel_requested_at=clock_timestamp(),cancel_acknowledged_at=CASE WHEN $2<>'running' THEN clock_timestamp() END,control_revision=control_revision+1,updated_at=clock_timestamp() WHERE id=$1`, id, next); err != nil {
		return ControlResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','task.cancel',jsonb_build_object('jobId',$1::text))`, id); err != nil {
		return ControlResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ControlResult{}, err
	}
	bounded, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		v, err := r.ControlStatus(ctx, id)
		if err != nil || !v.StopPending {
			return v, err
		}
		select {
		case <-bounded.Done():
			return v, nil
		case <-tick.C:
		}
	}
}

// Retry has a CAS admission revision and a durable checking state. Crashes in
// inspection leave checking (not runnable); repeating the same request cannot
// execute twice. Only a fresh explicit revision may inspect again after expiry.
func (r Repository) Retry(ctx context.Context, id string, revision int64, verify VerifyJob) (ControlResult, error) {
	var j Job
	var rev int64
	var live, stopPending bool
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ControlResult{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	err = tx.QueryRow(ctx, `SELECT id::text,kind,state,payload,control_revision,coalesce(lease_until>clock_timestamp(),false),cancel_requested_at IS NOT NULL AND cancel_acknowledged_at IS NULL FROM jobs WHERE id=$1 FOR UPDATE`, id).Scan(&j.ID, &j.Kind, &j.State, &j.Payload, &rev, &live, &stopPending)
	if err != nil {
		return ControlResult{}, err
	}
	if rev != revision {
		return ControlResult{}, ErrControlConflict
	}
	if (j.State == "succeeded" && j.Kind != "library.scan") || j.State == "pending" || j.State == "retry" {
		_ = tx.Rollback(ctx)
		return r.ControlStatus(ctx, id)
	}
	if live || stopPending {
		return ControlResult{}, errors.New("执行器尚未退出，不能重新执行外部操作")
	}
	if j.State == "checking" {
		var expired bool
		if err = tx.QueryRow(ctx, `SELECT updated_at<clock_timestamp()-interval '2 minutes' FROM jobs WHERE id=$1`, id).Scan(&expired); err != nil || !expired {
			return ControlResult{}, ErrControlConflict
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET state='checking',control_revision=control_revision+1,lease_token=NULL,lease_until=NULL,locked_by=NULL,updated_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		return ControlResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','task.retry_check',jsonb_build_object('jobId',$1::text,'revision',$2::bigint))`, id, rev+1); err != nil {
		return ControlResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ControlResult{}, err
	}
	bounded, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	var verified Verification
	var checkErr error
	if verify == nil {
		checkErr = errors.New("此任务没有安全核对适配器，禁止重放")
	} else {
		verified, checkErr = verify(bounded, j)
	}
	state, reason := "pending", verified.Reason
	if checkErr != nil {
		state = "unknown"
		reason = checkErr.Error()
	} else if verified.Complete {
		state = "succeeded"
	}
	done, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	tag, err := r.DB.Exec(done, `UPDATE jobs SET state=$3,last_error=CASE WHEN $3='unknown' THEN NULLIF($4,'') ELSE NULL END,attempts=CASE WHEN $3='pending' THEN 0 ELSE attempts END,cancel_requested_at=NULL,cancel_acknowledged_at=NULL,available_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1 AND state='checking' AND control_revision=$2`, id, rev+1, state, reason)
	if err != nil {
		return ControlResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return ControlResult{}, ErrControlConflict
	}
	if verified.NextJobID != "" && checkErr == nil {
		id = verified.NextJobID
	}
	out, err := r.ControlStatus(done, id)
	out.Reason = reason
	return out, err
}
func (r Repository) acknowledgeCancellation(ctx context.Context, j Job) error {
	// An interrupted handler may have committed an external effect. Never label
	// running cancellation as success or claim that the remote operation stopped.
	_, err := r.DB.Exec(ctx, `UPDATE jobs SET state=CASE WHEN kind IN('system.check','library.scan','search.site') THEN 'cancelled' ELSE 'unknown' END,cancel_acknowledged_at=clock_timestamp(),last_error='本地执行器已退出；外部副作用待核对',lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=clock_timestamp() WHERE id=$1 AND lease_token=$2 AND cancel_requested_at IS NOT NULL`, j.ID, j.LeaseToken)
	return err
}
func (r Repository) cancellationRequested(ctx context.Context, j Job) (bool, error) {
	var yes bool
	err := r.DB.QueryRow(ctx, `SELECT cancel_requested_at IS NOT NULL FROM jobs WHERE id=$1 AND lease_token=$2`, j.ID, j.LeaseToken).Scan(&yes)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrLeaseLost
	}
	return yes, err
}

// Side-effecting handlers cannot use lease expiry as permission to replay.
func NeedsVerification(kind string) bool {
	switch kind {
	case "files.batch", "download.add", "download.manual.add", "download.cleanup", "transfer.upload", "transfer.import.resume", "library.strm.repair", "library.artifacts.repair", "library.archive.cleanup", "library.refresh", "ops.backup", "ops.drill":
		return true
	}
	return false
}
func (r Repository) recoverControlled(ctx context.Context) error {
	if _, err := r.DB.Exec(ctx, `WITH stale AS(SELECT id FROM jobs WHERE state='checking' AND updated_at<clock_timestamp()-interval '2 minutes' FOR UPDATE SKIP LOCKED LIMIT 100) UPDATE jobs j SET state='unknown',last_error='核对执行器失联，外部状态仍未知',updated_at=clock_timestamp() FROM stale s WHERE j.id=s.id`); err != nil {
		return err
	}
	_, err := r.DB.Exec(ctx, `WITH expired AS(SELECT id FROM jobs WHERE state='running' AND lease_until<clock_timestamp() AND (cancel_requested_at IS NOT NULL OR kind IN('files.batch','download.add','download.manual.add','download.cleanup','transfer.upload','transfer.import.resume','library.strm.repair','library.artifacts.repair','library.archive.cleanup','library.refresh','ops.backup','ops.drill')) ORDER BY lease_until FOR UPDATE SKIP LOCKED LIMIT 100) UPDATE jobs j SET state='unknown',last_error='执行器租约已失效；外部状态待核对，不自动重放',cancel_acknowledged_at=CASE WHEN cancel_requested_at IS NOT NULL THEN clock_timestamp() END,lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=clock_timestamp() FROM expired e WHERE j.id=e.id`)
	return err
}
