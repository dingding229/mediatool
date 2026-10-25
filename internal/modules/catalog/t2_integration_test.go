package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
)

func t2DB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated test database required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "catalog_t2_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"cloud_files", "storage_connections", "media_assets", "media_asset_bindings", "media_items", "directory_classification", "native_tmdb_details", "media_probe_results", "integrations", "media_metadata_overrides"} {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, table}.Sanitize()+` (LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := db.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
		db.Close()
	})
	if _, e = db.Exec(ctx, `INSERT INTO directory_classification(singleton)VALUES(true)`); e != nil {
		t.Fatal(e)
	}
	return ctx, db
}
func t2Exec(t *testing.T, ctx context.Context, db *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, e := db.Exec(ctx, q, args...); e != nil {
		t.Fatal(e)
	}
}
func TestT2CategorySQLMatchesOrderedPolicy(t *testing.T) {
	ctx, db := t2DB(t)
	policies := []classification.Policy{classification.Defaults(), {Groups: []classification.Group{
		{ID: "off", Name: "停用", Directory: "off", MediaType: "movie", Enabled: false},
		{ID: "genre", Name: "片", Directory: "片", MediaType: "movie", Enabled: true, GenreIDs: []int{16}, Rules: []classification.Rule{{ID: "off-r", Name: "停用", Directory: "停用", Enabled: false}, {ID: "cn", Name: "重复", Directory: "中", Enabled: true, Countries: []string{"CN", "TW"}, Languages: []string{"zh"}, GenreIDs: []int{18}}, {ID: "other", Name: "重复", Directory: "余", Enabled: true}}},
		{ID: "plain", Name: "片", Directory: "plain", MediaType: "movie", Enabled: true},
	}}, {Groups: []classification.Group{}}}
	items := []metadata.Item{{MediaType: "movie", GenreIDs: []int{16, 18}, Countries: []string{"TW"}, OriginalLanguage: "zh"}, {MediaType: "movie", GenreIDs: []int{16}, Countries: []string{"CN"}, OriginalLanguage: "en"}, {MediaType: "movie", GenreIDs: []int{18}, Countries: []string{"US"}, OriginalLanguage: "en"}, {MediaType: "series", GenreIDs: []int{16}, Countries: []string{"JP"}}, {MediaType: "series", Countries: []string{"HK"}}, {MediaType: "movie"}, {MediaType: "unknown"}}
	for _, p := range policies {
		if e := p.Validate(); e != nil {
			t.Fatal(e)
		}
		for _, i := range items {
			raw, _ := json.Marshal(i)
			args := []any{raw, i.MediaType}
			expr := categorySQL(p, &args)
			var got string
			if e := db.QueryRow(ctx, `WITH m AS(SELECT $1::jsonb metadata,$2::text media_type)SELECT `+expr+` FROM m`, args...).Scan(&got); e != nil {
				t.Fatal(e)
			}
			want, _ := categoryFor(p, i)
			if got != want {
				t.Fatalf("ordered category mismatch %s != %s", got, want)
			}
		}
	}
	opts := categoryOptions(policies[1])
	if len(opts) != 3 || opts[0].ID != "r:genre:cn" || opts[1].Name != "片 / 重复" || opts[2].ID != "g:plain" {
		t.Fatal(opts)
	}
}
func TestT2CategoryFilterBeforePaginationAndManagedFacts(t *testing.T) {
	ctx, db := t2DB(t)
	e := Engine{DB: db}
	storage, work := jobs.UUID(), jobs.UUID()
	t2Exec(t, ctx, db, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, storage)
	t2Exec(t, ctx, db, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'movie',100,'Synthetic','2026','{"genreIds":[18],"countries":["US"],"originalLanguage":"en","overview":"stored"}')`, work)
	assets := []string{jobs.UUID(), jobs.UUID(), jobs.UUID()}
	files := []string{jobs.UUID(), jobs.UUID(), jobs.UUID()}
	for i, a := range assets {
		t2Exec(t, ctx, db, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,$3,7960117968)`, files[i], storage, "/fixture/"+a+".mkv")
		t2Exec(t, ctx, db, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, a, files[i])
		t2Exec(t, ctx, db, `INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, a, work)
	}
	t2Exec(t, ctx, db, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,fetched_at)VALUES('movie',100,'zh-CN','{"communityRating":6.681,"overview":"cached"}',now()),('movie',100,'en-US','{"communityRating":9}',now())`)
	t2Exec(t, ctx, db, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,result,probed_at)VALUES($1,'fixture','fixture','{"schema_version":1,"streams":[{"type":"Video","width":1920,"height":872}]}',now())`, files[0])
	// A different category must be excluded before LIMIT/OFFSET and count.
	otherWork, otherFile, otherAsset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	t2Exec(t, ctx, db, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',999,'Other','{"originalLanguage":"zh"}')`, otherWork)
	t2Exec(t, ctx, db, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/fixture/other.mkv',1)`, otherFile, storage)
	t2Exec(t, ctx, db, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, otherAsset, otherFile)
	t2Exec(t, ctx, db, `INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, otherAsset, otherWork)
	page, err := e.ListClassified(ctx, "", "", "", "r:movie:movie-west", 2, 1)
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.Items[0].Category != "电影 / 欧美电影" {
		t.Fatal(page, err)
	}
	detail, err := e.ManagedDetail(ctx, assets[0])
	if err != nil || detail.Rating == nil || *detail.Rating != 6.681 || detail.Size == nil || *detail.Size != 7960117968 || len(detail.Resolutions) != 1 || detail.Resolutions[0] != (Dimensions{1920, 872}) || detail.Overview != "cached" || detail.AssetCount != 1 || detail.Aggregate {
		t.Fatal(detail, err)
	}
	// Another version never borrows the first version's technical snapshot.
	second, err := e.ManagedDetail(ctx, assets[1])
	if err != nil || len(second.Resolutions) != 0 {
		t.Fatal(second, err)
	}
	if _, err = e.ListClassified(ctx, "", "", "", "movie-genre-injection", 1, 24); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err = e.ManagedDetail(ctx, "../escape"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	// A changed policy is reflected immediately without rewriting files/metadata.
	p := classification.Defaults()
	p.Groups[1].Rules[5].Name = "自定欧美分类"
	raw, _ := json.Marshal(p)
	t2Exec(t, ctx, db, `UPDATE directory_classification SET value=$1`, raw)
	renamed, err := e.ManagedDetail(ctx, assets[0])
	if err != nil || renamed.Category != "电影 / 自定欧美分类" {
		t.Fatal(renamed, err)
	}
	// Series/season totals include each physical file exactly once.
	t2Exec(t, ctx, db, `UPDATE media_items SET media_type='series',tmdb_id=101 WHERE id=$1`, work)
	t2Exec(t, ctx, db, `UPDATE media_asset_bindings SET season=1,episode=1 WHERE media_id=$1`, work)
	for _, id := range []string{"series_" + work, "season_" + work + "_1"} {
		v, err := e.ManagedDetail(ctx, id)
		if err != nil || v.AssetCount != 3 || v.Size == nil || *v.Size != 3*7960117968 || !v.Aggregate || len(v.Resolutions) != 1 {
			t.Fatal(v, err)
		}
	}
	// Malformed dimensions are ignored, not cast errors or guessed resolutions.
	t2Exec(t, ctx, db, `UPDATE media_probe_results SET result='{"schema_version":1,"streams":[{"type":"Video","width":"bad","height":"bad"}]}' WHERE cloud_file_id=$1`, files[0])
	if v, err := e.ManagedDetail(ctx, assets[0]); err != nil || len(v.Resolutions) != 0 {
		t.Fatal(v, err)
	}
}
