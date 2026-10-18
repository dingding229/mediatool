package subscriptions

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
)

// DeleteRevision atomically binds the destructive request to the observed revision.
func (e Engine) DeleteRevision(ctx context.Context, id string, revision int64) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM subscriptions WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&current); err != nil {
		return err
	}
	if current != revision {
		return errors.New("subscription revision changed")
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_runs WHERE subscription_id=$1 AND state IN('pending','running'))`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return errors.New("subscription active")
	}
	tag, err := tx.Exec(ctx, `UPDATE subscriptions SET archived_at=now(),enabled=false,revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$2`, id, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return dbutil.Commit(ctx, tx)
}
func (e Engine) HistoryPage(ctx context.Context, id string, start, limit int) ([]HistoryRun, error) {
	if start < 0 || start > 200000 || limit < 1 || limit > 200 {
		return nil, errors.New("invalid page")
	}
	if _, err := e.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := e.DB.Query(ctx, `SELECT id::text,preview,state,created_at,finished_at FROM subscription_runs WHERE subscription_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, id, limit, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryRun{}
	for rows.Next() {
		var v HistoryRun
		if err = rows.Scan(&v.ID, &v.Preview, &v.State, &v.CreatedAt, &v.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
