package library

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
)

// Called from the existing cron reconciler. A ready asset must be refreshed
// when its revision, Emby connection or checking user changed. Requests do not
// count as visibility; Confirm performs the real path-level check.
func (e Engine) ScheduleEmby(ctx context.Context) error {
	rows, err := e.DB.Query(ctx, `SELECT a.id::text,a.revision,i.revision,coalesce(p.value->>'embyUserId','') FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN integrations i ON i.kind='emby' AND i.enabled LEFT JOIN application_settings p ON p.singleton LEFT JOIN library_emby_checks c ON c.asset_id=a.id WHERE a.artifacts_state='ready' AND f.state='present' AND EXISTS(SELECT 1 FROM strm_entries s WHERE s.asset_id=a.id) AND (c.asset_id IS NULL OR c.asset_revision<>a.revision OR c.integration_revision<>i.revision OR c.user_id<>coalesce(p.value->>'embyUserId','') OR (c.state<>'visible' AND c.checked_at<now()-interval '10 minutes')) ORDER BY a.created_at LIMIT 100`)
	if err != nil {
		return err
	}
	type item struct {
		id                   string
		revision, connection int64
		user                 string
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.id, &x.revision, &x.connection, &x.user); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, x := range items {
		payload, _ := json.Marshal(map[string]string{"assetId": x.id})
		key := "auto-emby:" + x.id + ":" + string(mustJSON([]any{x.revision, x.connection, x.user}))
		if _, err = e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO UPDATE SET state='pending',attempts=0,available_at=now(),last_error=NULL WHERE jobs.state IN('failed','succeeded') AND jobs.updated_at<now()-interval '10 minutes'`, jobs.UUID(), payload, key); err != nil {
			return err
		}
	}
	return nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
