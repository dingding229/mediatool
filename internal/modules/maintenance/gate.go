package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/platform/workgate"
)

// BeginWork shares the same lightweight admission fence as background workers.
func BeginWork(ctx context.Context, db *pgxpool.Pool) (func(), error) {
	return workgate.BeginWork(ctx, db)
}

// CheckExternalWriteCommit uses the caller's own business transaction, not a
// second pool slot. QueueWebRestore takes the conflicting row lock before it
// queues a restore; the executor also takes it when pausing. A transaction
// that lost that race never returns a successful receipt for a concurrent undo.
func CheckExternalWriteCommit(ctx context.Context, tx pgx.Tx) error {
	var paused, restoring bool
	if err := tx.QueryRow(ctx, `SELECT paused FROM public.ops_mode WHERE singleton FOR SHARE`).Scan(&paused); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.ops_web_restores WHERE state IN('pending','running','unknown'))`).Scan(&restoring); err != nil {
		return err
	}
	if paused || restoring {
		return ErrConflict
	}
	return nil
}
