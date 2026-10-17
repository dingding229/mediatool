package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"media-tool/migrations"
)

const (
	userOne   = "11111111-0000-4000-8000-000000000055"
	userTwo   = "22222222-0000-4000-8000-000000000055"
	deviceOne = "33333333-0000-4000-8000-000000000055"
	storageID = "44444444-0000-4000-8000-000000000055"
	cloudID   = "55555555-0000-4000-8000-000000000055"
	assetID   = "66666666-0000-4000-8000-000000000055"
	sessionID = "77777777-0000-4000-8000-000000000055"
)

type nativeFixture struct {
	ctx      context.Context
	db       *sql.DB
	provider *goose.Provider
}

func newNativeFixture(t *testing.T) nativeFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated TEST_DATABASE_URL required")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || config.Database != "media_tool_test" {
		t.Fatal("only media_tool_test is allowed")
	}
	schema := pgx.Identifier{fmt.Sprintf("native_t03_%d", time.Now().UnixNano())}.Sanitize()
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	// Every connection uses ONLY this disposable schema; never Down public tables.
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error("close test database:", err)
		}
	})
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated database guard", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if _, err := db.ExecContext(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("remove owned test schema:", err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 54); err != nil {
		t.Fatal("migrate isolated schema to 54:", err)
	}
	return nativeFixture{ctx, db, provider}
}

func (f nativeFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	result, err := f.db.ExecContext(f.ctx, query, args...)
	if err != nil {
		t.Fatalf("fixture SQL failed: %s: %v", query, err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected < 1 {
		t.Fatalf("fixture SQL did not affect its target rows: %s: affected=%d error=%v", query, affected, err)
	}
}
func (f nativeFixture) integer(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRowContext(f.ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func (f nativeFixture) text(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := f.db.QueryRowContext(f.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func (f nativeFixture) rejects(t *testing.T, code, query string, args ...any) {
	t.Helper()
	_, err := f.db.ExecContext(f.ctx, query, args...)
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != code {
		t.Fatalf("expected SQLSTATE %s, got %v", code, err)
	}
}
func (f nativeFixture) version(t *testing.T, expected int64) {
	t.Helper()
	value, err := f.provider.GetDBVersion(f.ctx)
	if err != nil || value != expected {
		t.Fatalf("version=%d, want=%d: %v", value, expected, err)
	}
}

func TestNativeSchemaUpConstraintsDownUp(t *testing.T) {
	f := newNativeFixture(t)
	f.exec(t, `INSERT INTO media_users(id,username,display_name,password_hash,credential_version) VALUES($1,'t03_one','Synthetic user one','synthetic-hash',7),($2,'t03_two','Synthetic user two','synthetic-hash-two',2)`, userOne, userTwo)
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'movies',true),($1,'series',false),($2,'series',true)`, userOne, userTwo)
	t.Run("lock timeout rolls the entire migration back without losing grants", func(t *testing.T) {
		f.db.SetMaxOpenConns(2)
		defer f.db.SetMaxOpenConns(1)
		blocker, err := f.db.BeginTx(f.ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := blocker.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Error("release blocking transaction:", err)
			}
		}()
		if _, err := blocker.ExecContext(f.ctx, `LOCK TABLE media_user_library_grants IN SHARE MODE`); err != nil {
			t.Fatal(err)
		}
		_, err = f.provider.UpTo(f.ctx, 55)
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != "55P03" {
			t.Fatal("expected bounded migration lock failure", err)
		}
		f.version(t, 54)
		if f.integer(t, `SELECT count(*) FROM media_user_library_grants`) != 3 {
			t.Fatal("failed migration lost grants")
		}
		if f.integer(t, `SELECT (to_regclass('native_item_seq') IS NULL)::int`) != 1 {
			t.Fatal("failed migration leaked schema")
		}
	})
	if t.Failed() {
		return
	}
	t.Run("Up preserves accounts and stages legacy grants", func(t *testing.T) {
		if _, err := f.provider.UpTo(f.ctx, 55); err != nil {
			t.Fatal(err)
		}
		f.version(t, 55)
		if n := f.integer(t, `SELECT count(*) FROM media_user_legacy_grants`); n != 3 {
			t.Fatal("legacy count", n)
		}
		if n := f.integer(t, `SELECT count(*) FROM media_user_legacy_grants WHERE (user_id=$1 AND media_type='movie' AND can_play) OR (user_id=$1 AND media_type='series' AND NOT can_play) OR (user_id=$2 AND media_type='series' AND can_play)`, userOne, userTwo); n != 3 {
			t.Fatal("grant values changed")
		}
		if n := f.integer(t, `SELECT count(*) FROM media_user_library_grants`); n != 0 {
			t.Fatal("old grants not moved")
		}
		if n := f.integer(t, `SELECT count(*) FROM media_users WHERE id=$1 AND username='t03_one' AND password_hash='synthetic-hash' AND credential_version=7 AND max_streams=2 AND max_parental_rating IS NULL AND expires_at IS NULL`, userOne); n != 1 {
			t.Fatal("account fields/defaults changed")
		}
		if n := f.integer(t, `SELECT count(*) FROM native_libraries`); n != 0 {
			t.Fatal("migration invented category libraries")
		}
		if n := f.integer(t, `SELECT count(*) FROM native_server_settings WHERE singleton AND server_id::text ~ '^[0-9a-f]{32}$' AND server_name='Media Tool' AND version='4.8.10.0' AND NOT enabled AND NOT public_user_list AND revision=1`); n != 1 {
			t.Fatal("server defaults")
		}
		id := f.text(t, `SELECT server_id FROM native_server_settings`)
		results, err := f.provider.UpTo(f.ctx, 55)
		if err != nil || len(results) != 0 {
			t.Fatal("second Up was not idempotent", err)
		}
		if f.text(t, `SELECT server_id FROM native_server_settings`) != id || f.integer(t, `SELECT count(*) FROM media_user_legacy_grants`) != 3 {
			t.Fatal("second Up changed identity or grants")
		}
	})
	if t.Failed() {
		return
	}
	ids := map[string]int64{}
	for _, entity := range []string{"library", "movie", "series", "season", "episode"} {
		ids[entity] = f.integer(t, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES($1,'synthetic-identity')RETURNING id`, entity)
	}
	library, episode, movie := ids["library"], ids["episode"], ids["movie"]
	t.Run("global IDs are unique and retired keys stay reserved", func(t *testing.T) {
		if library != 100000 || ids["episode"] != library+4 {
			t.Fatal("entities do not share sequence", ids)
		}
		f.rejects(t, "23505", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('movie','synthetic-identity')`)
		f.exec(t, `UPDATE native_item_ids SET retired_at=now() WHERE id=$1`, ids["season"])
		f.rejects(t, "23505", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('season','synthetic-identity')`)
		if next := f.integer(t, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('season','second-season')RETURNING id`); next <= ids["episode"] {
			t.Fatal("ID was reused")
		}
		f.rejects(t, "23514", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('invalid','x')`)
		f.rejects(t, "23514", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('movie','')`)
		f.rejects(t, "23514", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('movie',$1)`, strings.Repeat("x", 513))
		f.rejects(t, "23502", `INSERT INTO native_item_ids(entity_type,internal_key)VALUES(NULL,'x')`)
	})
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type)VALUES('g:synthetic',$1,'synthetic','Synthetic library','Movies','movies')`, library)
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'g:synthetic',true)`, userOne)
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'Synthetic storage','clouddrive2')`, storageID)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/synthetic/file.mkv',1)`, cloudID, storageID)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, assetID, cloudID)
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)VALUES($1,'g:synthetic','classification',1,1)`, assetID)
	f.exec(t, `INSERT INTO media_user_devices(id,user_id,device_key)VALUES($1,$2,'synthetic-device')`, deviceOne, userOne)
	f.exec(t, `INSERT INTO media_user_tokens(token_digest,user_id,device_id,credential_version,expires_at)VALUES($1,$2,$3,7,now()+interval '1 day')`, []byte(strings.Repeat("x", 32)), userOne, deviceOne)
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id)VALUES($1,$2)`, userOne, episode)
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,fingerprint)VALUES($1,'size:1;mtime:2026-01-01')`, cloudID)
	f.exec(t, `INSERT INTO media_stills(item_id)VALUES($1)`, episode)
	f.exec(t, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id)VALUES($1,$2,$3,$4,'synthetic-source')`, sessionID, userOne, deviceOne, movie)

	t.Run("all declared enum values persist", func(t *testing.T) {
		for _, value := range []string{"movies", "tvshows"} {
			f.exec(t, `UPDATE native_libraries SET collection_type=$1`, value)
		}
		for _, value := range []string{"classification", "output_directory"} {
			f.exec(t, `UPDATE native_asset_libraries SET source=$1`, value)
		}
		for _, value := range []string{"pending", "probing", "ready", "failed"} {
			f.exec(t, `UPDATE media_probe_results SET state=$1,probing_started_at=CASE WHEN $1='probing' THEN now() END,probed_at=now(),result='{"format":{"duration":"42"}}'`, value)
		}
		for _, value := range []string{"pending", "grabbing", "ready", "failed", "skipped"} {
			f.exec(t, `UPDATE media_stills SET state=$1,grabbing_started_at=CASE WHEN $1='grabbing' THEN now() END,relative_path='episodes/synthetic.jpg',digest=repeat('a',64),width=320,height=180`, value)
		}
		for _, value := range []string{"playing", "paused", "stopped"} {
			f.exec(t, `UPDATE native_playback_sessions SET state=$1,ended_at=CASE WHEN $1='stopped' THEN now() END`, value)
		}
	})
	t.Run("invalid values and ownership fail with exact SQLSTATE", func(t *testing.T) {
		cases := []struct {
			name, code, sql string
			args            []any
		}{
			{"collection", "23514", `UPDATE native_libraries SET collection_type='series'`, nil},
			{"library ID", "23514", `UPDATE native_libraries SET id='g:wrong'`, nil},
			{"library directory traversal", "23514", `UPDATE native_libraries SET directory='../escape'`, nil},
			{"library entity", "23503", `UPDATE native_libraries SET item_id=$1`, []any{movie}},
			{"grant FK", "23503", `INSERT INTO media_user_library_grants(user_id,library_id)VALUES($1,'missing')`, []any{userOne}},
			{"source", "23514", `UPDATE native_asset_libraries SET source='other'`, nil},
			{"asset FK", "23503", `UPDATE native_asset_libraries SET asset_id='00000000-0000-4000-8000-000000000055'`, nil},
			{"policy revision", "23514", `UPDATE native_asset_libraries SET policy_revision=0`, nil},
			{"legacy type", "23514", `UPDATE media_user_legacy_grants SET media_type='all'`, nil},
			{"max streams low", "23514", `UPDATE media_users SET max_streams=0`, nil},
			{"max streams high", "23514", `UPDATE media_users SET max_streams=21`, nil},
			{"parental rating", "23514", `UPDATE media_users SET max_parental_rating=-1`, nil},
			{"device key length", "23514", `UPDATE media_user_devices SET device_key=repeat('x',257)`, nil},
			{"device duplicate", "23505", `INSERT INTO media_user_devices(id,user_id,device_key)VALUES(gen_random_uuid(),$1,'synthetic-device')`, []any{userOne}},
			{"token digest", "23514", `UPDATE media_user_tokens SET token_digest='short'::bytea`, nil},
			{"token ownership", "23503", `UPDATE media_user_tokens SET user_id=$1`, []any{userTwo}},
			{"token version", "23514", `UPDATE media_user_tokens SET credential_version=0`, nil},
			{"token expiry", "23514", `UPDATE media_user_tokens SET expires_at=created_at`, nil},
			{"user data position", "23514", `UPDATE media_user_item_data SET position_ticks=-1`, nil},
			{"user data play count", "23514", `UPDATE media_user_item_data SET play_count=-1`, nil},
			{"user data item FK", "23503", `UPDATE media_user_item_data SET item_id=99999`, nil},
			{"probe state", "23514", `UPDATE media_probe_results SET state='unknown'`, nil},
			{"probe timeout timestamp", "23514", `UPDATE media_probe_results SET state='probing',probing_started_at=NULL`, nil},
			{"probe fingerprint", "23514", `UPDATE media_probe_results SET fingerprint=''`, nil},
			{"probe attempts", "23514", `UPDATE media_probe_results SET attempts=-1`, nil},
			{"probe result object", "23514", `UPDATE media_probe_results SET result='[]'`, nil},
			{"probe result bound", "23514", `UPDATE media_probe_results SET result=jsonb_build_object('text',repeat('x',4194304))`, nil},
			{"probe ready receipt", "23514", `UPDATE media_probe_results SET state='ready',probed_at=NULL`, nil},
			{"still state", "23514", `UPDATE media_stills SET state='unknown'`, nil},
			{"still timeout timestamp", "23514", `UPDATE media_stills SET state='grabbing',grabbing_started_at=NULL`, nil},
			{"still must be episode", "23503", `UPDATE media_stills SET item_id=$1`, []any{movie}},
			{"still ready receipt", "23514", `UPDATE media_stills SET state='ready',digest=''`, nil},
			{"still digest", "23514", `UPDATE media_stills SET digest='invalid'`, nil},
			{"still width", "23514", `UPDATE media_stills SET width=-1`, nil},
			{"session state", "23514", `UPDATE native_playback_sessions SET state='unknown'`, nil},
			{"session ended timestamp", "23514", `UPDATE native_playback_sessions SET ended_at=NULL`, nil},
			{"session ownership", "23503", `UPDATE native_playback_sessions SET user_id=$1`, []any{userTwo}},
			{"session position", "23514", `UPDATE native_playback_sessions SET position_ticks=-1`, nil},
			{"server ID", "23514", `UPDATE native_server_settings SET server_id=repeat('A',32)`, nil},
			{"server singleton", "23514", `UPDATE native_server_settings SET singleton=false`, nil},
			{"server name length", "23514", `UPDATE native_server_settings SET server_name=repeat('x',101)`, nil},
			{"server version", "23514", `UPDATE native_server_settings SET version='4.8.bad'`, nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) { f.rejects(t, tc.code, tc.sql, tc.args...) })
		}
		for _, path := range []string{"../escape.jpg", "/absolute.jpg", "one/../escape.jpg", "one..jpg", `one\escape.jpg`, "C:/escape.jpg", "one\nfile.jpg", strings.Repeat("x", 1025)} {
			t.Run("still path "+path[:min(len(path), 30)], func(t *testing.T) { f.rejects(t, "23514", `UPDATE media_stills SET relative_path=$1`, path) })
		}
	})
	t.Run("hidden libraries retain grants and download defaults to false", func(t *testing.T) {
		f.exec(t, `UPDATE native_libraries SET hidden=true`)
		if n := f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1 AND can_play AND NOT can_download`, userOne); n != 1 {
			t.Fatal("grant changed", n)
		}
		f.rejects(t, "23503", `DELETE FROM native_libraries WHERE id='g:synthetic'`)
	})
	t.Run("device deletion cascades only device tokens and playback sessions", func(t *testing.T) {
		f.exec(t, `DELETE FROM media_user_devices WHERE id=$1`, deviceOne)
		if f.integer(t, `SELECT count(*) FROM media_user_tokens`) != 0 || f.integer(t, `SELECT count(*) FROM native_playback_sessions`) != 0 {
			t.Fatal("device rows survived")
		}
		if f.integer(t, `SELECT count(*) FROM media_users`) != 2 || f.integer(t, `SELECT count(*) FROM media_user_legacy_grants`) != 3 {
			t.Fatal("device deletion damaged accounts/grants")
		}
	})
	t.Run("indexes and recovery timestamps exist", func(t *testing.T) {
		for _, index := range []string{"native_libraries_visible_order", "media_user_library_grants_library", "native_asset_libraries_library", "media_user_devices_user_seen", "media_user_tokens_active_user", "media_user_tokens_device", "media_user_tokens_expiry", "media_user_item_data_item", "media_user_item_data_resume", "media_user_item_data_favorites", "media_user_item_data_history", "media_probe_results_due", "media_probe_results_stale", "media_stills_due", "media_stills_stale", "native_playback_sessions_active_user", "native_playback_sessions_stale", "native_playback_sessions_device", "native_playback_sessions_item"} {
			if f.integer(t, `SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, index) != 1 {
				t.Error("missing index", index)
			}
		}
		for _, table := range []string{"media_user_tokens", "native_playback_sessions"} {
			var definition string
			index := "media_user_tokens_active_user"
			if table == "native_playback_sessions" {
				index = "native_playback_sessions_active_user"
			}
			definition = f.text(t, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, index)
			if !strings.Contains(definition, "WHERE") {
				t.Error("active index is not partial", definition)
			}
		}
	})
	t.Run("Down restores structural baseline and unexpanded legacy grants", func(t *testing.T) {
		if _, err := f.provider.Down(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.version(t, 54)
		if f.integer(t, `SELECT count(*) FROM media_user_library_grants`) != 3 || f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1 AND library_id='series' AND NOT can_play`, userOne) != 1 {
			t.Fatal("legacy grants changed on Down")
		}
		if f.integer(t, `SELECT count(*) FROM media_users`) != 2 || f.integer(t, `SELECT count(*) FROM cloud_files`) != 1 || f.integer(t, `SELECT count(*) FROM media_assets`) != 1 {
			t.Fatal("preexisting account/media rows changed")
		}
		for _, table := range []string{"native_item_ids", "native_libraries", "media_user_legacy_grants", "native_asset_libraries", "native_server_settings", "media_user_devices", "media_user_tokens", "media_user_item_data", "media_probe_results", "media_stills", "native_playback_sessions", "native_item_seq"} {
			if n := f.integer(t, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname=$1`, table); n != 0 {
				t.Error("Down left relation", table)
			}
		}
		if n := f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name='media_users' AND column_name IN ('max_streams','max_parental_rating','expires_at')) OR (table_name='media_user_library_grants' AND column_name='can_download'))`); n != 0 {
			t.Fatal("Down left columns")
		}
	})
	if t.Failed() {
		return
	}
	t.Run("Up after Down is executable and migrates legacy grants again", func(t *testing.T) {
		if _, err := f.provider.UpTo(f.ctx, 55); err != nil {
			t.Fatal(err)
		}
		f.version(t, 55)
		if f.integer(t, `SELECT count(*) FROM media_user_legacy_grants`) != 3 || f.integer(t, `SELECT count(*) FROM media_user_library_grants`) != 0 {
			t.Fatal("reapplied grant migration")
		}
		if f.text(t, `SELECT password_hash FROM media_users WHERE id=$1`, userOne) != "synthetic-hash" {
			t.Fatal("password hash changed")
		}
	})
}
