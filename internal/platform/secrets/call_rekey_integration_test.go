package secrets

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"os"
	"strings"
	"testing"
)

func TestPluginCallRekeyRetainsOriginalAAD(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("test database only")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "call_rekey_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e = base.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
	}()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for _, c := range CipherColumns {
		table := pgx.Identifier{c.Table}.Sanitize()
		col := pgx.Identifier{c.Column}.Sanitize()
		if _, e = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (id text PRIMARY KEY)`); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, `ALTER TABLE `+table+` ADD COLUMN `+col+` bytea`); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = db.Exec(ctx, `CREATE TABLE audit_logs(actor text,action text,details jsonb)`); e != nil {
		t.Fatal(e)
	}
	old, _ := New(strings.Repeat("a", 64))
	next, _ := New(strings.Repeat("b", 64))
	a, _ := old.Seal("plugin:v2:call-args:retained", map[string]string{"secret": "synthetic"})
	b, _ := old.Seal("plugin:v2:call-result:retained", map[string]string{"data": "retained"})
	if _, e = db.Exec(ctx, `INSERT INTO plugin_v2_runs(id,args_ciphertext,result_ciphertext)VALUES('retained',$1,$2)`, a, b); e != nil {
		t.Fatal(e)
	}
	for _, dry := range []bool{true, false} {
		counts, e := Rekey(ctx, db, old, next, dry)
		if e != nil || counts["plugin_v2_runs"] != 1 || counts["plugin_v2_runs.result_ciphertext"] != 1 {
			t.Fatal(counts, e)
		}
		var x, y []byte
		if e = db.QueryRow(ctx, `SELECT args_ciphertext,result_ciphertext FROM plugin_v2_runs`).Scan(&x, &y); e != nil {
			t.Fatal(e)
		}
		vault := next
		if dry {
			vault = old
		}
		if _, e = vault.Open("plugin:v2:call-args:retained", x); e != nil {
			t.Fatal(e)
		}
		if _, e = vault.Open("plugin:v2:call-result:retained", y); e != nil {
			t.Fatal(e)
		}
	}
}

func TestT15MasterRekeyRevokesJWTOnlyOnCommit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	base, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "t15_rekey_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(t.Context(), `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	defer base.Exec(context.Background(), `DROP SCHEMA `+q+` CASCADE`)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(t.Context(), `CREATE TABLE external_api_keys(id uuid PRIMARY KEY,revoked_at timestamptz);CREATE TABLE audit_logs(actor text,action text,details jsonb);CREATE TABLE plugin_v2_kv(id uuid PRIMARY KEY,ciphertext bytea);INSERT INTO external_api_keys(id)VALUES('11111111-2222-4333-8444-555555555555')`); e != nil {
		t.Fatal(e)
	}
	for _, spec := range CipherColumns {
		table := pgx.Identifier{spec.Table}.Sanitize()
		column := pgx.Identifier{spec.Column}.Sanitize()
		if _, e = db.Exec(t.Context(), `CREATE TABLE IF NOT EXISTS `+table+`(id text PRIMARY KEY)`); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(t.Context(), `ALTER TABLE `+table+` ADD COLUMN IF NOT EXISTS `+column+` bytea`); e != nil {
			t.Fatal(e)
		}
	}
	old, _ := New(strings.Repeat("a", 64))
	next, _ := New(strings.Repeat("b", 64))
	id := jobs.UUID()
	encrypted, e := old.Seal("plugin:v2:kv:"+id, map[string]string{"fixture": "owned"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(t.Context(), `INSERT INTO plugin_v2_kv VALUES($1,$2)`, id, encrypted); e != nil {
		t.Fatal(e)
	}
	for _, dry := range []bool{true, false} {
		if _, e = Rekey(t.Context(), db, old, next, dry); e != nil {
			t.Fatal(e)
		}
		var revoked bool
		if e = db.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL FROM external_api_keys`).Scan(&revoked); e != nil || revoked == dry {
			t.Fatal(revoked, dry, e)
		}
	}
	if _, e = db.Exec(t.Context(), `UPDATE external_api_keys SET revoked_at=NULL;INSERT INTO plugin_v2_kv VALUES('99999999-2222-4333-8444-555555555555','bad')`); e != nil {
		t.Fatal(e)
	}
	if _, e = Rekey(t.Context(), db, next, old, false); e == nil {
		t.Fatal("invalid ciphertext committed")
	}
	var active bool
	if e = db.QueryRow(t.Context(), `SELECT revoked_at IS NULL FROM external_api_keys`).Scan(&active); e != nil || !active {
		t.Fatal("failed rotation revoked JWT", e)
	}
}
