package maintenance

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Maintenance history must not disappear when restoring an older application
// snapshot. Copy only this module's current catalog, never media/auth records.
func (e Engine) copyCurrentMetadata(ctx context.Context, clone *pgxpool.Pool) error {
	tx, err := clone.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `TRUNCATE ops_drills,ops_backups,ops_requests,ops_releases,ops_samples,ops_alerts,ops_policy,ops_mode`); err != nil {
		return err
	}
	for _, table := range []string{"ops_policy", "ops_mode", "ops_releases", "ops_backups", "ops_drills", "ops_requests", "ops_samples", "ops_alerts"} {
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
	if _, err = tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('ops_samples','id'),coalesce((SELECT max(id)FROM ops_samples),1),EXISTS(SELECT 1 FROM ops_samples))`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
