package backupverify

import (
	"context"
	"errors"
)

// Only the owner-approved v89 retirement may omit old external credential material.
// Call history's original columns and every unrelated table remain in the proof.
func externalJWTProjection(before map[string]Table) map[string]Table {
	out := map[string]Table{}
	for k, v := range before {
		if k != "external_api_keys" {
			out[k] = v
		}
	}
	return out
}
func verifyExternalJWTMigration(ctx context.Context, db Querier, before map[string]Table) (int64, error) {
	var removed, remaining, obsolete int64
	if e := db.QueryRow(ctx, `SELECT legacy_count FROM external_api_migration_receipt WHERE singleton`).Scan(&removed); e != nil {
		return 0, e
	}
	if e := db.QueryRow(ctx, `SELECT count(*)FROM external_api_keys`).Scan(&remaining); e != nil {
		return 0, e
	}
	if e := db.QueryRow(ctx, `SELECT count(*)FROM information_schema.columns WHERE table_schema='public'AND table_name='external_api_keys'AND column_name IN('token_digest','token_ciphertext','prefix','expires_at')`).Scan(&obsolete); e != nil {
		return 0, e
	}
	if removed != before["external_api_keys"].Rows || remaining != 0 || obsolete != 0 {
		return 0, errors.New("external JWT retirement proof mismatch")
	}
	return removed, nil
}
