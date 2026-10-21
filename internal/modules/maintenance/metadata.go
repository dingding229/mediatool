package maintenance

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
)

// Maintenance history must not disappear when restoring an older application
// snapshot. Copy only this module's current catalog, never media/auth records.
func (e Engine) copyCurrentMetadata(ctx context.Context, clone *pgxpool.Pool) error {
	tx, err := clone.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `TRUNCATE ops_web_restores,ops_request_verifications,ops_drills,ops_backups,ops_requests,ops_releases,ops_mode`); err != nil {
		return err
	}
	for _, table := range restoreMetadataTables {
		rows, err := e.DB.Query(ctx, `SELECT * FROM `+pgx.Identifier{table}.Sanitize())
		if err != nil {
			return err
		}
		columns := []string{}
		for _, field := range rows.FieldDescriptions() {
			columns = append(columns, field.Name)
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{table}, columns, rows)
		rows.Close()
		if err != nil {
			return err
		}
		if rows.Err() != nil {
			return rows.Err()
		}
	}
	return tx.Commit(ctx)
}
