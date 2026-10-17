package secrets

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"testing"
)

func TestPostgresRekeyDryRunAndRollback(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
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
		t.Fatal("test DB required")
	}
	db.Exec(ctx, `TRUNCATE integrations,resources,storage_connections,site_accounts,notification_channels,cloud_files,media_assets CASCADE`)
	defer db.Exec(ctx, `TRUNCATE integrations,resources,storage_connections,site_accounts,notification_channels,cloud_files,media_assets CASCADE`)
	old, _ := New(strings.Repeat("a", 64))
	next, _ := New(strings.Repeat("b", 64))
	id := jobs.UUID()
	cipher, _ := old.Seal(id, map[string]string{"token": "owned-secret"})
	if _, err = db.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'tmdb','fixture',$2)`, id, cipher); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{true, false} {
		if _, err = Rekey(ctx, db, old, next, dry); err != nil {
			t.Fatal(err)
		}
		var result []byte
		db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, id).Scan(&result)
		vault := next
		if dry {
			vault = old
		}
		values, err := vault.Open(id, result)
		if err != nil || values["token"] != "owned-secret" {
			t.Fatal("rotation lost secret", err)
		}
	}
	// Any invalid ciphertext aborts the whole transaction, including prior rows.
	broken := jobs.UUID()
	db.Exec(ctx, `INSERT INTO notification_channels(id,name,kind,credentials_ciphertext)VALUES($1,'broken','telegram',$2)`, broken, []byte("bad"))
	if _, err = Rekey(ctx, db, next, old, false); err == nil {
		t.Fatal("partial rekey committed")
	}
	var result []byte
	db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, id).Scan(&result)
	if _, err = next.Open(id, result); err != nil {
		t.Fatal("rollback lost new ciphertext")
	}
}

func TestPostgresRekeyIncludesScopedOfficialAuthorizationPayloads(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var databaseName string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil || databaseName != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	if _, err = db.Exec(ctx, `TRUNCATE integrations,resources,storage_connections,site_accounts,notification_channels,cloud_files,media_assets,administrator_accounts CASCADE`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `TRUNCATE integrations,resources,storage_connections,site_accounts,notification_channels,cloud_files,media_assets,administrator_accounts CASCADE`)
	old, _ := New(strings.Repeat("a", 64))
	next, _ := New(strings.Repeat("b", 64))
	connection, owner, active, completed := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	credentials, _ := old.Seal(connection, map[string]string{"accessToken": "fixture-access", "refreshToken": "fixture-refresh"})
	if _, err = db.Exec(ctx, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES($1,'rotation_fixture','fixture')`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'open115','rotation fixture',$2)`, connection, credentials); err != nil {
		t.Fatal(err)
	}
	payload, _ := old.Seal("open115-auth:"+active, map[string]string{"uid": "fixture-device", "sign": "fixture-sign", "verifier": "fixture-verifier"})
	for _, row := range []struct {
		id, state string
		data      []byte
	}{{active, "pending", payload}, {completed, "authorized", []byte{}}} {
		if _, err = db.Exec(ctx, `INSERT INTO open115_authorizations(id,integration_id,owner_account_id,owner_version,integration_revision,state,payload_ciphertext,expires_at)VALUES($1,$2,$3,1,1,$4,$5,now()+interval '5 minutes')`, row.id, connection, owner, row.state, row.data); err != nil {
			t.Fatal(err)
		}
	}
	for _, dry := range []bool{true, false} {
		counts, err := Rekey(ctx, db, old, next, dry)
		if err != nil {
			t.Fatal(err)
		}
		if counts["open115_authorizations"] != 1 {
			t.Fatal("scoped payload missing from rotation", counts)
		}
		var encrypted []byte
		if err = db.QueryRow(ctx, `SELECT payload_ciphertext FROM open115_authorizations WHERE id=$1`, active).Scan(&encrypted); err != nil {
			t.Fatal(err)
		}
		vault := next
		if dry {
			vault = old
		}
		decoded, err := vault.Open("open115-auth:"+active, encrypted)
		if err != nil || decoded["verifier"] != "fixture-verifier" {
			t.Fatal("wrong associated-data namespace", err)
		}
		var terminal []byte
		db.QueryRow(ctx, `SELECT payload_ciphertext FROM open115_authorizations WHERE id=$1`, completed).Scan(&terminal)
		if len(terminal) != 0 {
			t.Fatal("erased payload recreated")
		}
	}
	// A broken new-table payload must roll back earlier credential-table writes.
	if _, err = db.Exec(ctx, `UPDATE open115_authorizations SET payload_ciphertext=$2 WHERE id=$1`, active, []byte("corrupt")); err != nil {
		t.Fatal(err)
	}
	if _, err = Rekey(ctx, db, next, old, false); err == nil {
		t.Fatal("partial rotation committed")
	}
	var saved []byte
	db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, connection).Scan(&saved)
	if _, err = next.Open(connection, saved); err != nil {
		t.Fatal("old tables changed despite rollback")
	}
}

func TestPostgresRekeyIncludesScopedSearchSnapshots(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated DB required", err)
	}
	clean := `TRUNCATE integrations,resources,storage_connections,site_accounts,notification_channels,cloud_files,media_assets,search_runs CASCADE`
	if _, err = db.Exec(ctx, clean); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, clean)
	old, _ := New(strings.Repeat("a", 64))
	next, _ := New(strings.Repeat("b", 64))
	account, resource, run, snapshot := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	credentials, _ := old.Seal(account, map[string]string{"cookie": "fixture"})
	global, _ := old.Seal(resource, map[string]string{"url": "fixture-global"})
	hint, _ := old.Seal("search-run:"+snapshot, map[string]string{"url": "fixture-run-key"})
	if _, err = db.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'ptsite','fixture',$2)`, account, credentials); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,'42','Owned',$3)`, resource, account, global); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, run); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO search_run_resources(id,run_id,resource_id,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at)VALUES($1,$2,$3,'{}',$4,1,'fixture',now())`, snapshot, run, resource, hint); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{true, false} {
		counts, err := Rekey(ctx, db, old, next, dry)
		if err != nil || counts["search_run_resources"] != 1 || len(counts) != existingCipherColumns(t, ctx, db) {
			t.Fatal("run-scoped hint omitted from master-key rotation", counts, err)
		}
		var cipher []byte
		if err = db.QueryRow(ctx, `SELECT download_hint_ciphertext FROM search_run_resources WHERE id=$1`, snapshot).Scan(&cipher); err != nil {
			t.Fatal(err)
		}
		vault := next
		if dry {
			vault = old
		}
		values, err := vault.Open("search-run:"+snapshot, cipher)
		if err != nil || values["url"] != "fixture-run-key" {
			t.Fatal("snapshot AAD/secret lost", err)
		}
	}
}

func existingCipherColumns(t *testing.T, ctx context.Context, db *pgxpool.Pool) int {
	t.Helper()
	n := 0
	for _, c := range CipherColumns {
		var exists bool
		if e := db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, c.Table).Scan(&exists); e != nil {
			t.Fatal(e)
		}
		if exists {
			n++
		}
	}
	return n
}
