package backupverify

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The owner-authorized v37 transformation is narrowly scoped. Every other
// subscription field (identity, enabled, coverage, promotion/HR rules, etc.)
// and every unrelated table must retain its exact pre-migration checksum.
func subscriptionMigrationProjection(ctx context.Context, db *pgxpool.Pool, baseline map[string]Table) (map[string]Table, error) {
	out := map[string]Table{}
	for _, name := range []string{"subscriptions", "application_settings"} {
		old, ok := baseline[name]
		if !ok {
			continue
		}
		parts := []string{}
		for _, c := range old.Columns {
			if name == "subscriptions" && (c == "revision" || c == "interval_seconds" || c == "next_run_at" || c == "updated_at") {
				continue
			}
			if name == "application_settings" && (c == "revision" || c == "updated_at") {
				continue
			}
			expression := pgx.Identifier{c}.Sanitize()
			if name == "subscriptions" && c == "rules" {
				expression = `(rules-'minSize'-'maxSize'-'minSeeders') AS rules`
			}
			if name == "application_settings" && c == "value" {
				expression = `(value-'subscriptionIntervalMinutes') AS value`
			}
			parts = append(parts, expression)
		}
		value := Table{Columns: old.Columns}
		q := "SELECT count(*),coalesce(md5(string_agg(to_jsonb(t)::text,E'\n' ORDER BY to_jsonb(t)::text)),md5('')) FROM (SELECT " + join(parts) + " FROM " + pgx.Identifier{"public", name}.Sanitize() + ") t"
		if err := db.QueryRow(ctx, q).Scan(&value.Rows, &value.Digest); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}
func validateSubscriptionMigration(ctx context.Context, db *pgxpool.Pool) error {
	var bad int
	err := db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE interval_seconds<>(SELECT (value->>'subscriptionIntervalMinutes')::integer*60 FROM application_settings WHERE singleton) OR rules->>'minSize'<>'0' OR rules->>'maxSize'<>'0' OR rules->>'minSeeders'<>'0' OR NOT(rules ?& array['minSize','maxSize','minSeeders'])`).Scan(&bad)
	if err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("subscription migration contains %d unexpected rows", bad)
	}
	return nil
}
