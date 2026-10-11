package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/secrets"
	"media-tool/migrations"
)

func TestSnapshotMigration20To22KeepsLegacyCipherAndAllowsStructuralRollback(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated TEST_DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var name string
	if err = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	// Always return the disposable schema to the candidate version, even when a
	// negative assertion fails. No real business database is connected here.
	defer func() { goose.UpContext(ctx, db, ".") }()
	if err = goose.DownToContext(ctx, db, ".", 20); err != nil {
		t.Fatal(err)
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,search_runs,download_tasks,jobs,media_items CASCADE`
	if _, err = db.ExecContext(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	account, resource, run, task := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	credentials, _ := vault.Seal(account, map[string]string{"cookie": "fixture-only"})
	hint, _ := vault.Seal(resource, map[string]string{"url": "https://owned.invalid/download.php?id=42&passkey=FIXTURE-KEY"})
	if _, err = db.ExecContext(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'ptsite','Owned fixture',$2)`, account, credentials); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,'42','Owned',$3)`, resource, account, hint); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, run); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO search_run_resources(run_id,resource_id)VALUES($1,$2)`, run, resource); err != nil {
		t.Fatal(err)
	}
	originalDetails := json.RawMessage(`{"media":{"id":1,"mediaType":"movie"},"expectedHash":"fixture-hash"}`)
	if _, err = db.ExecContext(ctx, `INSERT INTO download_tasks(id,resource_id,details)VALUES($1,$2,$3)`, task, resource, originalDetails); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	var public []byte
	var cipher []byte
	if err = db.QueryRowContext(ctx, `SELECT public_snapshot,download_hint_ciphertext FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, run, resource).Scan(&public, &cipher); err != nil || public != nil || cipher != nil {
		t.Fatal("migration invented history from current mutable data", err)
	}
	var revision int64
	var required bool
	if err = db.QueryRowContext(ctx, `SELECT source_control_revision,source_recovery_required FROM download_tasks WHERE id=$1`, task).Scan(&revision, &required); err != nil || revision != 1 || required {
		t.Fatal("legacy task auto-paused or rebound", revision, required, err)
	}
	if err = goose.DownToContext(ctx, db, ".", 20); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT download_hint_ciphertext FROM resources WHERE id=$1`, resource).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	decoded, err := vault.Open(resource, cipher)
	if err != nil || decoded["url"] != "https://owned.invalid/download.php?id=42&passkey=FIXTURE-KEY" {
		t.Fatal("rollback modified original ciphertext", err)
	}
	var details []byte
	if err = db.QueryRowContext(ctx, `SELECT details FROM download_tasks WHERE id=$1`, task).Scan(&details); err != nil {
		t.Fatal(err)
	}
	var restored map[string]any
	if err = json.Unmarshal(details, &restored); err != nil || restored["expectedHash"] != "fixture-hash" {
		t.Fatal("rollback changed legacy task identity", err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
}
