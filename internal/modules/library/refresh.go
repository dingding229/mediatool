package library

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
)

func (e Engine) QueueRefresh(ctx context.Context, assetID string) error {
	var exists bool
	if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 AND a.artifacts_state='ready' AND f.state='present')`, assetID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("asset missing or outputs not ready")
	}
	payload, _ := json.Marshal(map[string]string{"assetId": assetID})
	_, err := e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "library.refresh:"+assetID+":"+jobs.UUID())
	return err
}
