package library

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"os"
	"strings"
	"testing"
)

func TestCommittedImportFactRollbackDedupeAndProjection(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test only")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated test database required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "f6_import_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := base.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
	}()
	if _, e = base.Exec(ctx, `CREATE TABLE `+q+`.outbox_events(LIKE public.outbox_events INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	candidate, asset := jobs.UUID(), jobs.UUID()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = recordImportTx(ctx, tx, candidate, asset); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&n); e != nil || n != 0 {
		t.Fatal("uncommitted fact visible", n, e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&n); e != nil || n != 0 {
		t.Fatal("rollback emitted fact", n, e)
	}
	for range 2 {
		tx, e = db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = recordImportTx(ctx, tx, candidate, asset); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var raw json.RawMessage
	if e = db.QueryRow(ctx, `SELECT payload FROM outbox_events`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var v map[string]string
	if e = json.Unmarshal(raw, &v); e != nil || len(v) != 3 || v["assetId"] != asset || v["operationId"] != candidate || v["state"] != "succeeded" {
		t.Fatal(string(raw), e)
	}
	if e = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate confirmed fact", n, e)
	}
}
