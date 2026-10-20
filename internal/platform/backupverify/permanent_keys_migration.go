package backupverify

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Only expiry removal is authorized by v49. ID, token/cipher/digest, permissions,
// revocation and audit usage must retain their exact bytes.
func permanentKeysProjection(ctx context.Context, db Querier, baseline map[string]Table) (map[string]Table, error) {
	old, ok := baseline["external_api_keys"]
	if !ok {
		return nil, nil
	}
	parts := []string{}
	for _, c := range old.Columns {
		if c != "expires_at" {
			parts = append(parts, pgx.Identifier{c}.Sanitize())
		}
	}
	value := Table{Columns: old.Columns}
	err := db.QueryRow(ctx, "SELECT count(*),coalesce(md5(string_agg(to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text)),md5('')) FROM (SELECT "+join(parts)+" FROM external_api_keys)t").Scan(&value.Rows, &value.Digest)
	return map[string]Table{"external_api_keys": value}, err
}
