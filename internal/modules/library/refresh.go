package library

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"strconv"
)

func refreshKey(assetID string, revision int64) string {
	return "library.native.refresh:" + assetID + ":" + strconv.FormatInt(revision, 10)
}

func (e Engine) QueueRefresh(ctx context.Context, assetID string) error {
	var exists bool
	var revision int64
	if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 AND a.artifacts_state='ready' AND f.state='present'),coalesce((SELECT revision FROM media_assets WHERE id=$1),0)`, assetID).Scan(&exists, &revision); err != nil {
		return err
	}
	if !exists {
		return errors.New("asset missing or outputs not ready")
	}
	payload, _ := json.Marshal(map[string]string{"assetId": assetID})
	key := refreshKey(assetID, revision)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO UPDATE SET state=CASE WHEN jobs.state IN('failed','retry','paused') THEN 'pending' ELSE jobs.state END,attempts=CASE WHEN jobs.state IN('failed','retry','paused') THEN 0 ELSE jobs.attempts END,available_at=CASE WHEN jobs.state IN('failed','retry','paused') THEN now() ELSE jobs.available_at END,last_error=CASE WHEN jobs.state IN('failed','retry','paused') THEN NULL ELSE jobs.last_error END`, jobs.UUID(), payload, key)
	if err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
