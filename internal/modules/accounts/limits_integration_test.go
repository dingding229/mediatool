package accounts

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestAuthenticationLimitCleanupIsIndexedAndBounded(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated database required", err)
	}
	if _, err = db.Exec(ctx, `TRUNCATE administrator_auth_limits;INSERT INTO administrator_auth_limits(scope_digest,window_start,attempts)SELECT decode(md5(n::text),'hex'),now()-interval '2 days',1 FROM generate_series(1,300)n`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `TRUNCATE administrator_auth_limits`)
	ok, err := (Repository{DB: db}).TakeAttempt(ctx, "owned-key", "owned-scope", 1)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var stale, indexes int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM administrator_auth_limits WHERE window_start<now()-interval '1 day'`).Scan(&stale); err != nil || stale != 44 {
		t.Fatal("cleanup was not limited to 256 rows", stale, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname='administrator_auth_limits_window'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatal("cleanup index missing", indexes, err)
	}
}
