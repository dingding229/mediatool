package catalog

import (
	"encoding/json"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"reflect"
	"testing"
)

func TestT2bSQLAndDetailUseIdenticalProviderFacts(t *testing.T) {
	ctx, db := t2DB(t)
	policy := classification.Defaults()
	base := metadata.Item{MediaType: "movie"}
	raws := []string{`{}`, `{"classification":null}`, `{"classification":{"version":2,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}}`, `{"classification":{"version":1,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}}`, `{"classification":{"version":1,"genreIds":[],"countries":[],"originalLanguage":""}}`, `{"classification":{"version":1,"genreIds":["18"],"countries":["TW"],"originalLanguage":"zh"}}`, `{"classification":{"version":1,"genreIds":[18],"countries":["secret/url"],"originalLanguage":"zh"}}`, `{"classification":{"version":1,"genreIds":[18],"countries":["TW"],"originalLanguage":"ZH"}}`}
	for _, country := range []string{"CN", "TW", "HK", "MO"} {
		for _, language := range []string{"zh", "cn", "yue"} {
			raw, _ := json.Marshal(map[string]any{"classification": classification.Facts{Version: 1, GenreIDs: []int{18}, Countries: []string{country}, OriginalLanguage: language}})
			raws = append(raws, string(raw))
		}
	}
	for _, kind := range []string{"movie", "series"} {
		base.MediaType = kind
		for _, raw := range raws {
			b, _ := json.Marshal(base)
			args := []any{b, raw, kind}
			sql := `WITH m AS(SELECT $1::jsonb metadata,$3::text media_type),d AS(SELECT $2::jsonb payload),c AS(SELECT ` + mergedFactsSQL + ` metadata FROM m,d) SELECT c.metadata,` + categorySQLFrom(policy, &args, "c.metadata") + ` FROM m,c`
			var merged []byte
			var category string
			if e := db.QueryRow(ctx, sql, args...).Scan(&merged, &category); e != nil {
				t.Fatalf("%s: %v", raw, e)
			}
			var got metadata.Item
			if e := json.Unmarshal(merged, &got); e != nil {
				t.Fatal(e)
			}
			want := classificationInput(base, []byte(raw))
			if !reflect.DeepEqual(append([]string{}, got.Countries...), append([]string{}, want.Countries...)) || got.OriginalLanguage != want.OriginalLanguage || !reflect.DeepEqual(append([]int{}, got.GenreIDs...), append([]int{}, want.GenreIDs...)) {
				t.Fatalf("facts diverged %s: %+v != %+v", raw, got, want)
			}
			wantCategory, _ := categoryFor(policy, want)
			if category != wantCategory {
				t.Fatal("list/detail diverged", raw, category, wantCategory)
			}
		}
	}
}
func TestT2bProviderClassificationFillsMissingIngestFactsBeforePaging(t *testing.T) {
	ctx, db := t2DB(t)
	storage := jobs.UUID()
	t2Exec(t, ctx, db, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, storage)
	assets := []string{}
	works := []string{}
	for i, kind := range []string{"movie", "series", "movie"} {
		work, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
		assets = append(assets, asset)
		works = append(works, work)
		basic := `{}`
		facts := `{"classification":{"version":1,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}}`
		if i == 1 {
			facts = `{"classification":{"version":1,"genreIds":[18],"countries":["HK"],"originalLanguage":"cn"}}`
		}
		if i == 2 {
			basic = `{"countries":["US"],"originalLanguage":"en"}`
			facts = `{"classification":{"version":1,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}}`
		}
		t2Exec(t, ctx, db, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,$2,$3,$4,$5)`, work, kind, 700+i, []string{"台湾电影", "香港剧", "美国电影"}[i], basic)
		t2Exec(t, ctx, db, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,$3,1)`, file, storage, "/fixture/"+asset+".mkv")
		t2Exec(t, ctx, db, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
		t2Exec(t, ctx, db, `INSERT INTO media_asset_bindings(asset_id,media_id,season,episode)VALUES($1,$2,1,1)`, asset, work)
		t2Exec(t, ctx, db, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,payload,fetched_at)VALUES($1,$2,$3,now())`, kind, 700+i, facts)
	}
	e := Engine{DB: db}
	for i, want := range []string{"电影 / 华语电影", "电视剧 / 国产剧", "电影 / 欧美电影"} {
		detail, err := e.ManagedDetail(ctx, assets[i])
		if err != nil || detail.Category != want {
			t.Fatal(detail, err)
		}
		p, err := e.ListClassified(ctx, "", "", "", detail.CategoryID, 1, 1)
		if err != nil || p.Total != 1 || len(p.Items) != 1 || p.Items[0].Category != want {
			t.Fatal(p, err)
		}
	}
	series, err := e.ManagedDetail(ctx, "series_"+works[1])
	if err != nil || series.Category != "电视剧 / 国产剧" {
		t.Fatal(series, err)
	}
	// A different language is not accidentally used as a newer authoritative cache.
	t2Exec(t, ctx, db, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,fetched_at)VALUES('movie',700,'en-US','{"classification":{"version":1,"genreIds":[18],"countries":["US"],"originalLanguage":"en"}}',now()+interval '1 day')`)
	if v, err := e.ManagedDetail(ctx, assets[0]); err != nil || v.Category != "电影 / 华语电影" {
		t.Fatal(v, err)
	}
}
