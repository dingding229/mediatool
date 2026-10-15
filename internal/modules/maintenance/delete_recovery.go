package maintenance

import (
	"context"
	"errors"
	"os"
	"time"
)

const deletionRecoveryAfter = 10 * time.Minute

type deletionPath struct {
	exists bool
	info   os.FileInfo
}

type deletionPaths struct {
	original, tombstone deletionPath
}

// Lstat distinguishes absence from IO/permission failures, and does not follow
// an unexpected symlink. Neither deletion nor recovery may treat errors as proof
// that the backup files are gone.
func inspectBackupDeletion(dir string, lstat func(string) (os.FileInfo, error)) (deletionPaths, error) {
	var paths deletionPaths
	for _, p := range []struct {
		path  string
		entry *deletionPath
	}{{dir, &paths.original}, {dir + ".deleting", &paths.tombstone}} {
		info, err := lstat(p.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return paths, err
		}
		p.entry.exists = true
		p.entry.info = info
	}
	return paths, nil
}

func (e Engine) backupDeletionRecoveryState(id string, started, now time.Time, lstat func(string) (os.FileInfo, error)) (string, string, error) {
	if started.After(now.Add(-deletionRecoveryAfter)) {
		return "", "", nil // Active intents must remain non-retryable.
	}
	dir, err := e.dir(id)
	if err != nil {
		return "failed", "删除任务超时，目录核对失败；请检查目录或权限后重新删除：" + err.Error(), err
	}
	paths, err := inspectBackupDeletion(dir, lstat)
	if err != nil {
		return "failed", "删除任务超时，目录核对失败；请检查目录或权限后重新删除：" + err.Error(), err
	}
	if !paths.original.exists && !paths.tombstone.exists {
		return "deleted", "删除任务超时后核对确认：备份目录和隔离目录均不存在，本模块备份已删除，业务媒体未删除", nil
	}
	if (paths.original.exists && (!paths.original.info.IsDir() || paths.original.info.Mode()&os.ModeSymlink != 0)) || (paths.tombstone.exists && (!paths.tombstone.info.IsDir() || paths.tombstone.info.Mode()&os.ModeSymlink != 0)) {
		return "failed", "删除任务超时：备份路径类型异常，需人工核对后重新删除；未自动删除文件", nil
	}
	if paths.original.exists && paths.tombstone.exists {
		return "failed", "删除任务超时：原始目录与隔离目录同时存在，需人工核对后再删除；未自动删除文件", nil
	}
	return "failed", "删除任务超时：备份目录或隔离目录仍存在，可重新删除；未自动删除文件", nil
}

// Recover only stale intents, in bounded batches on the existing cron path.
// Row locks and the observed attempt timestamp prevent competing recovery or
// delete requests from replacing the state inspected by this transaction.
func (e Engine) recoverDeletingBackups(ctx context.Context, now time.Time) (map[string]struct{}, error) {
	recovered := map[string]struct{}{}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return recovered, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return recovered, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,COALESCE(deletion_started_at,created_at) FROM ops_backups WHERE state='deleting' AND COALESCE(deletion_started_at,created_at)<=$1 ORDER BY COALESCE(deletion_started_at,created_at) LIMIT 100 FOR UPDATE SKIP LOCKED`, now.Add(-deletionRecoveryAfter))
	if err != nil {
		return recovered, err
	}
	type intent struct {
		id      string
		started time.Time
	}
	var intents []intent
	for rows.Next() {
		var item intent
		if err = rows.Scan(&item.id, &item.started); err != nil {
			rows.Close()
			return recovered, err
		}
		intents = append(intents, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return recovered, err
	}
	var reconcileErr error
	for _, item := range intents {
		state, message, inspectErr := e.backupDeletionRecoveryState(item.id, item.started, now, os.Lstat)
		if inspectErr != nil {
			// An unreadable path is not absence. Expose a retryable failure
			// and retain the diagnostic rather than claiming deletion succeeded.
			reconcileErr = errors.Join(reconcileErr, inspectErr)
		}
		if state == "" {
			continue
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE ops_backups SET state=$2,message=$3,completed_at=now() WHERE id=$1 AND state='deleting' AND COALESCE(deletion_started_at,created_at)=$4`, item.id, state, message, item.started)
		if updateErr != nil {
			return recovered, updateErr
		}
		if err = requireOneAffected(tag); err != nil {
			return recovered, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('system','ops.backup.delete_recovered',jsonb_build_object('id',$1::text,'state',$2::text))`, item.id, state); err != nil {
			return recovered, err
		}
		recovered[item.id] = struct{}{}
	}
	if err = tx.Commit(ctx); err != nil {
		return recovered, err
	}
	return recovered, reconcileErr
}

// An old cleanup may complete after a newer intent was started. Bind every
// completion (successful or not) to the timestamp allocated by that attempt.
func (e Engine) failBackupDeletion(ctx context.Context, id string, started time.Time, message string, cause error) error {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tag, err := e.DB.Exec(updateCtx, `UPDATE ops_backups SET state='failed',message=$2,completed_at=now() WHERE id=$1 AND state='deleting' AND deletion_started_at=$3`, id, message+cause.Error(), started)
	if err == nil {
		err = requireOneAffected(tag)
	}
	return errors.Join(cause, err)
}
