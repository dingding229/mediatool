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
