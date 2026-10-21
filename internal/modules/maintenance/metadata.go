package maintenance

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	"strings"
)

// Maintenance history must not disappear when restoring an older application
// snapshot. Copy only this module's current catalog, never media/auth records.
func (e Engine) copyCurrentMetadata(ctx context.Context, clone *pgxpool.Pool) error {
	tx, err := clone.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `TRUNCATE ops_web_restores,ops_request_verifications,ops_drills,ops_backups,ops_requests,ops_releases,ops_samples,ops_alerts,ops_policy,ops_mode`); err != nil {
		return err
	}
	for _, table := range []string{"ops_policy", "ops_mode", "ops_releases", "ops_backups", "ops_drills", "ops_requests", "ops_request_verifications", "ops_samples", "ops_alerts", "ops_web_restores"} {
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

var pluginPreservedTables = []string{"app_settings", "plugin_v2_definitions", "plugin_v2_instances", "plugin_v2_configs", "plugin_v2_kv", "plugin_v2_grants", "plugin_v2_operations", "plugin_v2_snapshots", "plugin_v2_extensions", "plugin_v2_schedules", "plugin_v2_event_journal", "plugin_v2_subscriptions", "plugin_v2_runs", "plugin_v2_api_keys", "plugin_v2_api_limits", "plugin_v2_manual_requests", "plugin_v2_state_snapshots", "plugin_v2_state_values", "plugin_v2_state_tasks", "plugin_v2_journal_retention"}

// Plugin authorization/configuration and workflow ledgers must never roll back
// independently from the app's live supervisors or filesystem. Keep this domain
// current, including encrypted KV (credentials), while restoring other history.
func (e Engine) copyCurrentPlugins(ctx context.Context, clone *pgxpool.Pool) error {
	tables := pluginPreservedTables
	names := []string{}
	for _, table := range tables {
		names = append(names, pgx.Identifier{table}.Sanitize())
	}
	tx, err := clone.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ",")); err != nil {
		return err
	}
	for _, table := range tables {
		rows, err := e.DB.Query(ctx, "SELECT * FROM "+pgx.Identifier{table}.Sanitize())
		if err != nil {
			return err
		}
		cols := []string{}
		for _, field := range rows.FieldDescriptions() {
			cols = append(cols, field.Name)
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{table}, cols, rows)
		rows.Close()
		if err != nil {
			return err
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('plugin_v2_event_journal','cursor'),coalesce((SELECT max(cursor)FROM plugin_v2_event_journal),1),EXISTS(SELECT 1 FROM plugin_v2_event_journal))`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
