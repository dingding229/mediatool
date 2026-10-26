package externalapi

import (
	"context"
	"media-tool/internal/platform/database"
	"media-tool/migrations"
	"os"
	"strings"
	"testing"
)

func TestExistingExpiredAndRevokedCredentialsBecomePermanentWithoutTokenChanges(t *testing.T) {
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
	_, err = tx.Exec(ctx, `ALTER TABLE external_api_keys DROP CONSTRAINT external_api_keys_permanent;
 INSERT INTO external_api_keys(id,name,token_digest,token_ciphertext,prefix,scopes,expires_at,revoked_at)VALUES('11111111-0000-4000-8000-000000000049','old key','digest','cipher','prefix','["status.read"]','2000-01-01',now());`)
	if err != nil {
		t.Fatal(err)
	}
	q := `SELECT md5((to_jsonb(k)-'expires_at')::text) FROM external_api_keys k WHERE id='11111111-0000-4000-8000-000000000049'`
	var before, after string
	if err = tx.QueryRow(ctx, q).Scan(&before); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.FS.ReadFile("00049_permanent_api_credentials.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(raw), "-- +goose Down")[0]
	if _, err = tx.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	var permanent bool
	tx.QueryRow(ctx, `SELECT expires_at IS NULL AND revoked_at IS NOT NULL FROM external_api_keys WHERE id='11111111-0000-4000-8000-000000000049'`).Scan(&permanent)
	if err = tx.QueryRow(ctx, q).Scan(&after); err != nil || before != after || !permanent {
		t.Fatal(before, after, permanent, err)
	}
}
