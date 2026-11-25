package maintenance

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/schema"
	"media-tool/migrations"
)

// Run only inside the fresh run-labelled PostgreSQL container created by the
// test harness. Never clone from the shared CI or replay database.
func TestRP33CLiveWebRestoreAndCompensation(t *testing.T) {
	run := os.Getenv("MEDIA_TOOL_RP33C_TEST_RUN")
	if run == "" {
		t.Skip("use scripts/test-web-restore-real.sh in an isolated Docker daemon")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Host != "127.0.0.1:5432" || u.Path != "/media_tool_test" || os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("refusing non-disposable or unlocked PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var dbName, marker string
	var empty bool
	if err := db.QueryRow(ctx, `SELECT current_database(),current_setting('mediatool.test_run',true),NOT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public')`).Scan(&dbName, &marker, &empty); err != nil || !empty || dbName != "media_tool_test" || marker != run {
		t.Fatal("database is not the new marked fixture", err)
	}
	if err = schema.Up(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err = db.QueryRow(ctx, SchemaSQL).Scan(&version); err != nil || version != buildinfo.Schema {
		t.Fatal("disposable fixture is not at the current migration", version, err)
	}
	e := Engine{DB: db, DSN: dsn, Root: t.TempDir(), OutputRoot: t.TempDir(), Key: strings.Repeat("9", 64), Revision: strings.Repeat("a", 40)}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	oldSettings := os.Getenv("MEDIA_TOOL_RP33C_PRE98") == "1"
	if oldSettings {
		// Use the exact Down DDL to reproduce pre-98 routine settings while
		// keeping the current schema ledger for the real engine's schema guard.
		raw, err := migrations.FS.ReadFile("00098_public_function_search_path.sql")
		if err != nil {
			t.Fatal(err)
		}
		_, down, ok := strings.Cut(string(raw), "-- +goose Down\n")
		if !ok {
			t.Fatal("missing migration Down")
		}
		exec(down)
		var missing int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace JOIN pg_language l ON l.oid=p.prolang WHERE n.nspname='public' AND p.prokind='f' AND l.lanname IN ('sql','plpgsql') AND NOT EXISTS(SELECT 1 FROM unnest(p.proconfig) AS s(setting) WHERE split_part(s.setting,'=',1)='search_path' AND split_part(s.setting,'=',2)<>'')`).Scan(&missing); err != nil || missing != 5 {
			t.Fatal("negative control did not restore all five unconfigured functions", missing, err)
		}
		t.Logf("pre-98 control: proconfig guard finds %d unconfigured public functions", missing)
	}

	connection, file, asset, media, output := newID(), newID(), newID(), newID(), newID()
	const relative = "owned.strm"
	const original = "https://owned.example/synthetic\n"
	exec(`INSERT INTO storage_connections(id,name,provider) VALUES($1,'fixture','clouddrive2')`, connection)
	exec(`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes) VALUES($1,$2,'/fixture/owned.mkv',1)`, file, connection)
	exec(`INSERT INTO media_assets(id,cloud_file_id) VALUES($1,$2)`, asset, file)
	exec(`INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata) VALUES($1,'movie',1,'Synthetic','2026','{}')`, media)
	exec(`INSERT INTO media_asset_bindings(asset_id,media_id) VALUES($1,$2)`, asset, media)
	exec(`INSERT INTO strm_entries(id,asset_id,output_path,key_reference) VALUES($1,$2,$3,'fixture')`, output, asset, relative)
	if err = os.WriteFile(filepath.Join(e.OutputRoot, relative), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if !oldSettings {
		// cloud_files' trigger is UPDATE/DELETE-only (not COPY INSERT). Exercise
		// its live lookup explicitly under the same empty path as pg_restore.
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('search_path','',true)`); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `UPDATE public.cloud_files SET state=state WHERE id=$1`, file)
		if err = errors.Join(err, tx.Rollback(ctx)); err != nil {
			t.Fatal("cloud_files trigger cannot use empty search_path", err)
		}
	}

	token := seedRP33NativeToken(t, ctx, e)
	// e.DB must remain open when the existing token fixture's cleanup runs.
	// A fresh login after the snapshot also proves tokens from after the backup
	// are not resurrected by either path.
	store := &nativeauth.Store{DB: db, Secret: e.Key}
	login := func(device string) string {
		t.Helper()
		got, err := store.Login(ctx, rp33User, "synthetic-password", contracts.DeviceInfo{DeviceID: device, ClientIP: "127.0.0.1"})
		if err != nil {
			t.Fatal("native fixture login failed", err)
		}
		return got.AccessToken
	}
	assertRejected := func(raw, device string) {
		t.Helper()
		if _, _, _, err := store.Authenticate(ctx, raw, contracts.DeviceInfo{DeviceID: device}); !errors.Is(err, contracts.ErrUnauthenticated) {
			t.Fatal("old native device token was not rejected", err)
		}
	}
	backup := func() string {
		t.Helper()
		id, err := e.QueueBackup(ctx, "manual")
		if err != nil {
			t.Fatal(err)
		}
		if err = e.Backup(ctx, id); err != nil {
			t.Fatal(err)
		}
		m, _, err := e.VerifyBundle(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"strm_entries", "media_asset_bindings", "cloud_files"} {
			if m.Tables[table].Rows != 1 {
				t.Fatal("backup does not contain populated trigger table", table, m.Tables[table].Rows)
			}
		}
		return id
	}
	assertResult := func(id, state, applied, title string) {
		t.Helper()
		got, err := e.WebRestore(ctx, id)
		if err != nil || got.State != state || got.SnapshotID == nil || got.AppliedBackup == nil || *got.AppliedBackup != applied {
			t.Fatal("unexpected restoration receipt", got.State, got.Message, err)
		}
		var stamp, visible string
		err = db.QueryRow(ctx, `SELECT c.backup_id::text,m.title FROM ops_restore_commit c JOIN media_asset_bindings b ON b.asset_id=$2 JOIN media_items m ON m.id=b.media_id JOIN media_assets a ON a.id=b.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id JOIN strm_entries s ON s.asset_id=a.id AND s.output_path=$3 WHERE c.request_id=$1`, id, asset, relative).Scan(&stamp, &visible)
		if err != nil || stamp != applied || visible != title {
			t.Fatal("wrong commit marker or incomplete trigger-table restore", stamp, visible, err)
		}
		raw, err := os.ReadFile(filepath.Join(e.OutputRoot, relative))
		if err != nil || string(raw) != original {
			t.Fatal("original output bytes changed", err)
		}
	}
	assertNoCandidates := func() {
		t.Helper()
		var count int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_database WHERE datname ~ '^media_tool_restore_'`).Scan(&count); err != nil || count != 0 {
			t.Fatal("temporary candidate databases did not clean up", count, err)
		}
	}
	target := backup()
	after := login("after-backup")
	exec(`UPDATE media_items SET title='Changed after backup' WHERE id=$1`, media)
	r := queueWeb(t, e, target)
	err = e.RunWebRestore(ctx)
	if oldSettings {
		got, lookupErr := e.WebRestore(ctx, r.ID)
		var stampCount int
		countErr := db.QueryRow(ctx, `SELECT count(*) FROM ops_restore_commit WHERE request_id=$1`, r.ID).Scan(&stampCount)
		if err == nil || lookupErr != nil || countErr != nil || got.State != "unknown" || got.SnapshotID == nil || got.AppliedBackup != nil || stampCount != 0 || strings.Count(got.Message, "exit status 3") != 2 {
			t.Fatal("pre-98 control did not fail both imports", got.State, got.Message, stampCount, err, lookupErr, countErr)
		}
		assertNoCandidates()
		t.Log("pre-98 control: real target and compensation both failed, no import receipt committed")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	assertResult(r.ID, "completed", target, "Synthetic")
	checkRP33NativeToken(t, ctx, e, token, true)
	assertRejected(after, "after-backup")
	assertNoCandidates()

	target = backup()
	live := login("before-compensation")
	exec(`UPDATE media_items SET title='Keep via compensation' WHERE id=$1`, media)
	r = queueWeb(t, e, target)
	var imports int
	e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		if name == "psql" {
			imports++
			if imports == 1 {
				in = io.MultiReader(in, strings.NewReader("\nSELECT 1/0;\n"))
			}
		}
		return runCommand(ctx, name, args, env, in, out)
	}
	if err = e.RunWebRestore(ctx); err != nil || imports != 2 {
		t.Fatal("expected real failed import and real compensation", imports, err)
	}
	got, err := e.WebRestore(ctx, r.ID)
	if err != nil || got.SnapshotID == nil {
		t.Fatal("compensation snapshot missing", err)
	}
	assertResult(r.ID, "rolled_back", *got.SnapshotID, "Keep via compensation")
	assertRejected(live, "before-compensation")
	var revoked bool
	if err := db.QueryRow(ctx, `SELECT t.revoked_at IS NOT NULL FROM media_user_tokens t JOIN media_user_devices d ON d.id=t.device_id WHERE d.device_key='before-compensation'`).Scan(&revoked); err != nil || !revoked {
		t.Fatal("compensation did not preserve device and atomically revoke its token", err)
	}
	assertNoCandidates()
	t.Log("real target and compensation committed independently; native tokens rejected and candidate DBs removed")
}
