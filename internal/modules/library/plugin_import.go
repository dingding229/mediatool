package library

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
)

// recordImportTx emits identity-only TransferComplete after all artifact evidence
// has been applied. A stable candidate fact key prevents duplicate recovery facts.
func recordImportTx(ctx context.Context, tx pgx.Tx, candidate, asset string) error {
	return outbox.EmitVersionTx(ctx, tx, "library.organized", asset, candidate+":confirmed:v1", contracts.TransferCompleted{OperationID: candidate, AssetID: asset, State: "succeeded"})
}
func (e Engine) commitImport(ctx context.Context, candidate, asset string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	tag, err := tx.Exec(ctx, `UPDATE library_candidates SET state='confirmed',updated_at=now() WHERE id=$1 AND asset_id=$2 AND state IN('artifacts_pending','confirmed') AND EXISTS(SELECT 1 FROM media_assets a WHERE a.id=$2 AND a.artifacts_state='ready') AND NOT EXISTS(SELECT 1 FROM media_artifact_journal j WHERE j.asset_id=$2 AND j.state<>'applied')`, candidate, asset)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrOutputLayoutRequired
	}
	if err = recordImportTx(ctx, tx, candidate, asset); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
