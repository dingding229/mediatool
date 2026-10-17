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
	rows, e := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('goose_db_version','application_settings','media_archive_policy','network_proxy_settings','native_media_libraries','native_server_settings','player_ua_policy','directory_classification','site_metric_schedule','ops_policy','ops_mode')`)
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
	cipher, _ := vault.Seal("plugin:restore-fixture", map[string]string{"apiKey": "owned"})
	state, _ := vault.Seal("plugin-data:restore-fixture", map[string]string{"data": "{\"count\":1}"})
	manifest := []byte(`{"id":"restore-fixture","name":"Owned","version":"1","runtime":"wasm"}`)
	module := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	_, e = db.Exec(ctx, `INSERT INTO plugin_legacy_wasm_plugins(id,name,version,manifest,config_ciphertext,plugin_data_ciphertext,wasm_module)VALUES('restore-fixture','Owned','1',$1,$2,$3,$4)`, manifest, cipher, state, module)
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
	db.QueryRow(ctx, `SELECT plugin_data_ciphertext FROM plugin_legacy_wasm_plugins WHERE id='restore-fixture'`).Scan(&after)
	if string(after) != string(state) {
		t.Fatal("dry-run modified state bytes")
	}
}
