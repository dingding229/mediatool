package backupverify

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestPermanentKeyProjectionProtectsEveryOtherCredentialField(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE external_api_keys(id uuid PRIMARY KEY,name text NOT NULL,token_digest bytea NOT NULL UNIQUE,token_ciphertext bytea NOT NULL,prefix text NOT NULL,scopes jsonb NOT NULL,expires_at timestamptz,revoked_at timestamptz,last_used_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),CONSTRAINT external_api_keys_permanent CHECK(expires_at IS NULL))ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_api_keys(id,name,token_digest,token_ciphertext,prefix,scopes)VALUES('22222222-0000-4000-8000-000000000049','projection','projection-digest','owned-cipher','prefix','["status.read"]')`)
	if err != nil {
		t.Fatal(err)
	}
	baseline := map[string]Table{"external_api_keys": {Columns: []string{"id", "name", "token_digest", "token_ciphertext", "prefix", "scopes", "expires_at", "revoked_at", "last_used_at", "created_at"}}}
	before, err := permanentKeysProjection(ctx, tx, baseline)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `ALTER TABLE external_api_keys DROP CONSTRAINT external_api_keys_permanent; UPDATE external_api_keys SET expires_at='2000-01-01' WHERE id='22222222-0000-4000-8000-000000000049'`)
	if err != nil {
		t.Fatal(err)
	}
	after, err := permanentKeysProjection(ctx, tx, baseline)
	if err != nil || !equal(before, after, false) {
		t.Fatal("expiry-only change changed projection", err)
	}
	tx.Exec(ctx, `UPDATE external_api_keys SET scopes='["downloads.run"]' WHERE id='22222222-0000-4000-8000-000000000049'`)
	after, err = permanentKeysProjection(ctx, tx, baseline)
	if err != nil || equal(before, after, false) {
		t.Fatal("permission tampering was not detected", err)
	}
}
