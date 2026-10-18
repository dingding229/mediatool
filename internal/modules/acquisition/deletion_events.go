package acquisition

import (
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"time"
)

// Called only after both provider absence and every owned local file's absence
// have been read back. Request/unknown paths cannot call this commit point.
func (e Engine) finishCleanupConfirmed(ctx context.Context, id string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var approved time.Time
	err = tx.QueryRow(ctx, `UPDATE download_cleanup_requests SET state='succeeded',message='已读回确认种子和登记下载文件均不存在；云端媒体和STRM保留',updated_at=now() WHERE task_id=$1 AND state IN('deleting','uncertain') RETURNING approved_at`, id).Scan(&approved)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE download_tasks SET seeding_state='removed',updated_at=now() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("download unavailable")
	}
	for _, typ := range []string{"download.files.deleted", "download.task.deleted"} {
		if err = outbox.EmitVersionTx(ctx, tx, typ, id, approved.UTC().Format(time.RFC3339Nano), contracts.DownloadDeleted{TaskID: id, FilesDeleted: true}); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','download.cleanup_complete',jsonb_build_object('taskId',$1::text,'cloudMediaPreserved',true,'strmKeyPreserved',true))`, id); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
