package jobs

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/outbox"
	"time"
)

// Only a committed, lease-fenced terminal job produces a failure. Retry,
// paused/unknown uploads and recoverable journal conflicts alone emit nothing.
func terminalOrganizingFact(ctx context.Context, tx pgx.Tx, id string) error {
	var kind, state string
	var updated time.Time
	if e := tx.QueryRow(ctx, `SELECT kind,state,updated_at FROM jobs WHERE id=$1`, id).Scan(&kind, &state, &updated); e != nil {
		return e
	}
	if kind != "transfer.upload" || state != "failed" {
		return nil
	}
	// Exhausting a scheduler lease is not proof that an in-flight upload failed.
	// Preserve unknown/starting/uploading/reconciliation states for read-back.
	var transferState string
	e := tx.QueryRow(ctx, `SELECT t.state FROM jobs q JOIN transfer_tasks t ON t.id::text=q.payload->>'transferId' WHERE q.id=$1 FOR SHARE OF t`, id).Scan(&transferState)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	if transferState != "pending" && transferState != "verified" && transferState != "artifacts_pending" {
		return nil
	}
	version := updated.UTC().Format(time.RFC3339Nano)
	if e := outbox.EmitVersionTx(ctx, tx, "library.organize.failed", id, version, contracts.OrganizeFailure{OperationID: id, Kind: "transfer", ReasonCode: "attempts_exhausted"}); e != nil {
		return e
	}
	var asset string
	var count int
	e = tx.QueryRow(ctx, `SELECT t.download_file_id::text,count(j.id) FROM jobs q JOIN transfer_tasks t ON t.id::text=q.payload->>'transferId' JOIN media_artifact_journal j ON j.asset_id=t.download_file_id AND j.artifact_kind='subtitle' AND j.state<>'applied' WHERE q.id=$1 AND (j.state='conflict' OR EXISTS(SELECT 1 FROM outbox_events o WHERE o.event_type='subtitle.attempt.failed' AND o.payload->>'operationId'=j.id::text)) GROUP BY t.download_file_id`, id).Scan(&asset, &count)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	return outbox.EmitVersionTx(ctx, tx, "library.subtitles.failed", id, version, contracts.SubtitleOutcome{OperationID: id, AssetID: asset, Count: count, State: "failed"})
}
