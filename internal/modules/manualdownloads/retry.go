package manualdownloads

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/jobs"
)

var ErrRetryState = errors.New("手动下载任务尚不允许重新提交；结果未知时请先只读核对下载器")

func retryEligibility(state string, attempted, confirm, acceptDuplicateRisk bool) error {
	if !confirm {
		return errors.New("请明确确认重新提交手动下载任务")
	}
	switch state {
	case "blocked", "failed":
		if attempted {
			return ErrRetryState
		}
	case "absent_confirmed":
		if !acceptDuplicateRisk {
			return errors.New("已核对下载器未找到种子；重新提交前请确认可能的延迟接收/重复风险")
		}
	default:
		return ErrRetryState
	}
	return nil
}

// Retry only requeues a durable task; it never issues an add request itself.
// The same advisory key as submit prevents a retry racing an outbound attempt.
func (e Engine) Retry(ctx context.Context, id string, confirm, acceptDuplicateRisk bool) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,913))`, id).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return errors.New("手动任务正在提交或核对，请稍后重试")
	}
	var state string
	var attempted bool
	if err = tx.QueryRow(ctx, `SELECT state,submit_attempted FROM manual_download_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &attempted); err != nil {
		return err
	}
	if err = retryEligibility(state, attempted, confirm, acceptDuplicateRisk); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key=$1 AND state='running')`, "download.manual.add:"+id).Scan(&active); err != nil {
		return err
	}
	if active {
		return errors.New("原手动任务仍在运行，请等待任务结束后再重试")
	}
	if _, err = tx.Exec(ctx, `UPDATE manual_download_tasks SET state='pending',submit_attempted=false,message='管理员已确认重试，等待下载器核对',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"id": id})
	tag, err := tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'download.manual.add',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO UPDATE SET state='pending',attempts=0,available_at=now(),last_error=NULL,lease_token=NULL,lease_until=NULL,locked_by=NULL WHERE jobs.state<>'running'`, jobs.UUID(), payload, "download.manual.add:"+id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrRetryState
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','download.manual.retry',jsonb_build_object('taskId',$1::text,'duplicateRiskAccepted',$2::boolean))`, id, acceptDuplicateRisk); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Reconcile observes the old immutable endpoint/hash/path only. A confirmed
// absence is a separate state, not implicit permission to replay the add.
func (e Engine) Reconcile(ctx context.Context, id string) (string, error) {
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,913))`, id).Scan(&locked); err != nil {
		return "", err
	}
	if !locked {
		return "", errors.New("手动任务正在提交或核对，请稍后重试")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		conn.Exec(c, `SELECT pg_advisory_unlock(hashtextextended($1,913))`, id)
	}()
	var integration, hash, endpoint, save, category, state string
	var attempted bool
	if err = conn.QueryRow(ctx, `SELECT integration_id::text,info_hash,endpoint,save_path,category,state,submit_attempted FROM manual_download_tasks WHERE id=$1`, id).Scan(&integration, &hash, &endpoint, &save, &category, &state, &attempted); err != nil {
		return "", err
	}
	if state != "uncertain" && state != "submitting" && state != "absent_confirmed" && !(attempted && (state == "blocked" || state == "failed")) {
		return "", ErrRetryState
	}
	client, err := e.client(ctx, integration, endpoint)
	if err != nil {
		return "", err
	}
	task, err := client.Inspect(ctx, hash)
	if err == nil {
		if err = e.observe(ctx, id, task, save, category); err != nil {
			return "", err
		}
		if task.Complete {
			return "completed", nil
		}
		return "downloading", nil
	}
	if !errors.Is(err, qbittorrent.ErrNotFound) {
		return "", err
	}
	tag, err := conn.Exec(ctx, `UPDATE manual_download_tasks SET state='absent_confirmed',message='只读核对：下载器未找到此hash；未重新提交。确认风险后可重试',updated_at=now() WHERE id=$1 AND state=$2 AND submit_attempted`, id, state)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrRetryState
	}
	return "absent_confirmed", nil
}
