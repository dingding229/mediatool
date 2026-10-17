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

func TestArchivedPluginRekeyRetainsOriginalAAD(t *testing.T) {
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
	schema := "archive_rekey_" + strings.ReplaceAll(jobs.UUID(), "-", "")
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
		if c.Table == "extension_plugins" {
			continue
		}
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
	a, _ := old.Seal("plugin:archived", map[string]string{"secret": "synthetic"})
	b, _ := old.Seal("plugin-data:archived", map[string]string{"data": "retained"})
	if _, e = db.Exec(ctx, `INSERT INTO plugin_legacy_wasm_plugins(id,config_ciphertext,plugin_data_ciphertext)VALUES('archived',$1,$2)`, a, b); e != nil {
		t.Fatal(e)
	}
	for _, dry := range []bool{true, false} {
		counts, e := Rekey(ctx, db, old, next, dry)
		if e != nil || counts["plugin_legacy_wasm_plugins"] != 1 || counts["plugin_legacy_wasm_plugins.config_ciphertext"] != 1 {
			t.Fatal(counts, e)
		}
		var x, y []byte
		if e = db.QueryRow(ctx, `SELECT config_ciphertext,plugin_data_ciphertext FROM plugin_legacy_wasm_plugins`).Scan(&x, &y); e != nil {
			t.Fatal(e)
		}
		vault := next
		if dry {
			vault = old
		}
		if _, e = vault.Open("plugin:archived", x); e != nil {
			t.Fatal(e)
		}
		if _, e = vault.Open("plugin-data:archived", y); e != nil {
			t.Fatal(e)
		}
	}
}
