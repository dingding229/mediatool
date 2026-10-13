package backupverify

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/plugins"
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
	rows, e := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('goose_db_version','application_settings','media_archive_policy')`)
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
	packageRaw, e := os.ReadFile("../../../examples/plugins/automation.zip")
	if e != nil {
		t.Fatal(e)
	}
	fixture, e := plugins.ParsePackage(packageRaw)
	if e != nil {
		t.Fatal(e)
	}
	fixture.Manifest.ID = "restore-fixture"
	manifest, _ := json.Marshal(fixture.Manifest)
	_, e = db.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest,config_ciphertext,plugin_data_ciphertext,wasm_module)VALUES('restore-fixture','Owned','1',$1,$2,$3,$4)`, manifest, cipher, state, fixture.Module)
	if e != nil {
		t.Fatal(e)
	}
	report, e := Verify(ctx, db, dsn, "media_tool_test", strings.Repeat("9", 64), true)
	if e != nil || !report.DryRunCiphertextsUnchanged || !report.SecondMigrationIdempotent || !report.LegacyBusinessColumnsPreserved || report.FinalVersion != 36 || len(report.CipherRows) != len(secrets.CipherColumns) {
		t.Fatal(report, e)
	}
	var after []byte
	db.QueryRow(ctx, `SELECT plugin_data_ciphertext FROM extension_plugins WHERE id='restore-fixture'`).Scan(&after)
	if string(after) != string(state) {
		t.Fatal("dry-run modified state bytes")
	}
}
