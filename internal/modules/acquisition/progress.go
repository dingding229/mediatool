package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"media-tool/internal/modules/downloads"
	"time"
)

// Sync only observes existing torrents. It never re-adds a torrent or changes
// the download/import state, even when an operator presses Refresh.
func (e Engine) Sync(ctx context.Context) error {
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM download_tasks WHERE info_hash IS NOT NULL AND download_state NOT IN ('pending','failed','cancelled') ORDER BY coalesce((details->>'syncAttemptAt')::timestamptz,created_at) LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var results []error
	for _, id := range ids {
		if ctx.Err() != nil {
			return errors.Join(append(results, ctx.Err())...)
		}
		_, connection, hash, _, details, err := e.load(ctx, id)
		if err == nil {
			_, err = e.syncTask(ctx, id, connection, hash, details)
		}
		if err != nil {
			results = append(results, err)
		}
	}
	return errors.Join(results...)
}
func (e Engine) syncTask(ctx context.Context, id, connection, hash string, details taskDetails) (downloads.Task, error) {
	at := time.Now().UTC()
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client, err := e.client(bounded, connection, details)
	var task downloads.Task
	if err == nil {
		task, err = client.InspectAliases(bounded, hash, details.HashAliases)
	}
	patch := map[string]any{"syncAttemptAt": at, "syncError": ""}
	if err != nil {
		slog.Warn("download status synchronization failed", "module", "downloads", "taskId", id, "error", err.Error())
		patch["syncError"] = err.Error()
	} else {
		slog.Debug("download status observed", "module", "downloads", "taskId", id, "infoHash", task.ID, "progress", task.Progress, "qbState", task.State, "downloadSpeed", task.DownloadSpeed, "seeds", task.Seeds, "peers", task.Peers)
		patch["syncedAt"], patch["progress"], patch["qbState"] = at, task.Progress, task.State
		patch["downloadSpeed"], patch["uploadSpeed"], patch["seeds"], patch["peers"] = task.DownloadSpeed, task.UploadSpeed, task.Seeds, task.Peers
		trackers, trackerErr := client.Trackers(bounded, task.ID)
		patch["trackerError"] = ""
		if trackerErr == nil {
			patch["trackers"] = trackers
		} else {
			patch["trackerError"] = trackerErr.Error()
		}

	}
	raw, marshalErr := json.Marshal(patch)
	if marshalErr != nil {
		return task, marshalErr
	}
	// Record even a timed-out request, with a bounded detached context. Merge only
	// telemetry keys; preserve concurrent source/identity/control changes. A late
	// older response cannot replace a newer snapshot.
	saveCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer done()
	_, saveErr := e.DB.Exec(saveCtx, `UPDATE download_tasks SET details=details||$2::jsonb,seeding_state=CASE WHEN $4 THEN $5 ELSE seeding_state END,updated_at=now() WHERE id=$1 AND info_hash IS NOT NULL AND download_state NOT IN ('pending','failed','cancelled') AND (details->>'syncAttemptAt' IS NULL OR (details->>'syncAttemptAt')::timestamptz <= $3)`, id, raw, at, err == nil, func() string {
		if task.Seeding {
			return "seeding"
		}
		return "pending"
	}())
	return task, errors.Join(err, saveErr)
}
