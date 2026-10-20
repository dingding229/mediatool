package migrations_test

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"os"
	"testing"
)

func TestNativeLibraryGranularityMigrationAndCatalogSync(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 56); err != nil {
		t.Fatal(err)
	}
	f.version(t, 56)
	user := "88888888-0000-4000-8000-000000000056"
	f.exec(t, `INSERT INTO media_users(id,username,display_name,password_hash,credential_version)VALUES($1,'t11_user','Synthetic T11 user','synthetic-hash',1)`, user)

	policy := classification.Policy{Groups: []classification.Group{
		{ID: "movie", Name: "电影", Directory: "电影", MediaType: "movie", Enabled: true, Rules: []classification.Rule{{ID: "doc", Name: "纪录片", Directory: "纪录片", Enabled: true}}},
		{ID: "series", Name: "电视剧", Directory: "电视剧", MediaType: "series", Enabled: true, Rules: []classification.Rule{{ID: "series-doc", Name: "纪录片", Directory: "纪录片", Enabled: true}}},
	}}
	pool := catalogTestPool(t, f)
	defer pool.Close()
	repo := catalog.Repository{DB: pool}
	if err := repo.Sync(f.ctx, policy, 2, nativelibrary.GranularityRule); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play) SELECT $1,id,collection_type='movies' FROM native_libraries WHERE NOT hidden`, user)
	if f.integer(t, `SELECT count(*) FROM native_libraries WHERE NOT hidden`) != 2 {
		t.Fatal("visible libraries")
	}
	if f.integer(t, `SELECT count(*) FROM native_libraries WHERE name='纪录片 · 电影'`) != 1 || f.integer(t, `SELECT count(*) FROM native_libraries WHERE name='纪录片 · 电视剧'`) != 1 {
		t.Fatal("same-name disambiguation")
	}
	ids := map[string]string{}
	rows, err := f.db.QueryContext(f.ctx, `SELECT id,item_id::text FROM native_libraries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, item string
		if err := rows.Scan(&id, &item); err != nil {
			t.Fatal(err)
		}
		ids[id] = item
	}
	rows.Close()
	if err := repo.Sync(f.ctx, policy, 3, nativelibrary.GranularityRule); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_libraries`) != 2 || f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1`, user) != 2 {
		t.Fatal("repeat sync changed rows")
	}
	for i := range policy.Groups {
		if policy.Groups[i].ID == "movie" {
			policy.Groups[i].Rules[0].Enabled = false
		}
	}
	if err := repo.Sync(f.ctx, policy, 4, nativelibrary.GranularityRule); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_libraries WHERE id='r:movie:doc' AND hidden`) != 1 || f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1 AND library_id='r:movie:doc'`, user) != 1 {
		t.Fatal("hidden library or authorization lost")
	}
	policy.Groups[0].Rules[0].Enabled = true
	if err := repo.Sync(f.ctx, policy, 5, nativelibrary.GranularityRule); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT item_id::text FROM native_libraries WHERE id='r:movie:doc'`) != ids["r:movie:doc"] {
		t.Fatal("reactivation changed ID")
	}
	if err := repo.Sync(f.ctx, policy, 6, nativelibrary.GranularityGroup); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_libraries WHERE id IN('g:movie','g:series') AND NOT hidden`) != 2 || f.integer(t, `SELECT count(*) FROM native_libraries WHERE id LIKE 'r:%' AND NOT hidden`) != 0 {
		t.Fatal("group granularity")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1`, user) != 2 {
		t.Fatal("existing grants not retained")
	}
	if f.text(t, `SELECT library_granularity FROM native_server_settings WHERE singleton`) != "rule" {
		t.Fatal("default granularity")
	}
	f.rejects(t, "23514", `UPDATE native_server_settings SET library_granularity='invalid' WHERE singleton`)
	if _, err := f.provider.Down(f.ctx); err != nil {
		t.Fatal("down 00056", err)
	}
	if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='native_server_settings' AND column_name='library_granularity'`) != 0 {
		t.Fatal("down left granularity")
	}
	if _, err := f.provider.UpTo(f.ctx, 56); err != nil {
		t.Fatal("up 00056 after down", err)
	}
	if f.text(t, `SELECT library_granularity FROM native_server_settings WHERE singleton`) != "rule" {
		t.Fatal("reup default granularity")
	}
}

func catalogTestPool(t *testing.T, f nativeFixture) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.QueryRowContext(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.MaxConns = 2
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}
