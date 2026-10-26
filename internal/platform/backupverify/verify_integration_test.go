package backupverify

import (
	"context"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestPostgresCloneGuardAndCompleteRekeyMigrationImmutability(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB")
	}
	if _, e = Verify(ctx, db, dsn, "media_tool", strings.Repeat("9", 64), false); e == nil {
		t.Fatal("production allowed")
	}
	rows, e := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('goose_db_version','application_settings','media_archive_policy','network_proxy_settings','native_media_libraries','native_server_settings','player_ua_policy','player_ip_block_policy','directory_classification','site_metric_schedule','ops_policy','ops_mode')`)
	if e != nil {
		t.Fatal(e)
	}
	tables := []string{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		tables = append(tables, "\""+n+"\"")
	}
	rows.Close()
	cleanup := "TRUNCATE " + strings.Join(tables, ",") + " CASCADE"
	if _, e = db.Exec(ctx, cleanup); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("9", 64))
	cipher, _ := vault.Seal("11111111-0000-4000-8000-000000000068", map[string]string{"apiKey": "owned"})
	_, e = db.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext) VALUES('11111111-0000-4000-8000-000000000068','tmdb','restore fixture',$1)`, cipher)
	if e != nil {
		t.Fatal(e)
	}
	expectedColumns := 0
	for _, column := range secrets.CipherColumns {
		var exists bool
		if e = db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, column.Table).Scan(&exists); e != nil {
			t.Fatal(e)
		}
		if exists {
			expectedColumns++
		}
	}
	report, e := Verify(ctx, db, dsn, "media_tool_test", strings.Repeat("9", 64), true)
	if e != nil || !report.DryRunCiphertextsUnchanged || !report.SecondMigrationIdempotent || !report.LegacyBusinessColumnsPreserved || report.FinalVersion != buildinfo.Schema || len(report.CipherRows) != expectedColumns {
		t.Fatal(report, e)
	}
	var after []byte
	db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id='11111111-0000-4000-8000-000000000068'`).Scan(&after)
	if string(after) != string(cipher) {
		t.Fatal("dry-run modified state bytes")
	}
}
