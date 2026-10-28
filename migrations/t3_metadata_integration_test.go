package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/adapters/tmdb"
	"media-tool/internal/modules/jobs"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/nativetmdb"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	managed "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/modules/nativelibrary"
)

func t3Values(v map[string]any) metadataedit.Values {
	out := metadataedit.Values{}
	for k, x := range v {
		r, _ := json.Marshal(x)
		out[k] = r
	}
	return out
}
func TestT3MigrationRoundTrip(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 83); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.Down(f.ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.UpTo(f.ctx, 83); e != nil {
		t.Fatal(e)
	}
}
func TestT3ManualLocksRestoreConcurrencyAndNativeEffectiveSQL(t *testing.T) {
	f := newBrowseFixture(t)
	repo := metadataedit.Repository{DB: f.pool}
	m := f.media(t, 934001, "Original", "2025", `{"originalTitle":"Original title","overview":"Provider base","genres":["Drama"],"officialRating":"G","communityRating":5,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}`)
	a := f.asset(t, m, "g:test-a", time.Now(), "mkv")
	f.asset(t, m, "g:test-a", time.Now(), "mp4")
	r, e := repo.Read(f.ctx, a.asset)
	if e != nil {
		t.Fatal(e)
	}
	set := t3Values(map[string]any{"title": "Manual title", "originalTitle": "Manual Original", "sortName": "A manual sort", "overview": "Manual overview", "year": 2026, "premiereDate": "2026-05-01", "genres": []string{" Custom ", "Custom"}, "tags": []string{"Owned"}, "tagline": "Owned tagline", "communityRating": 6.66, "officialRating": "G"})
	r, e = repo.Save(f.ctx, a.asset, "test-admin", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: set})
	if e != nil {
		t.Fatal(e)
	}
	if r.Revision != 1 || len(r.Overrides) != 11 {
		t.Fatal(r)
	}
	// All normal writers can replace their own base, without writing the overlay.
	f.exec(t, `UPDATE media_items SET title='New scrape',metadata=metadata||'{"overview":"New scrape","communityRating":8.2}'::jsonb WHERE id=$1`, m)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,state)VALUES('movie',934001,'zh-CN','pending')`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":934001,"title":"Fetched title","overview":"Newest scrape","vote_average":9,"original_language":"zh","production_countries":[{"iso_3166_1":"TW"}],"genres":[{"id":18,"name":"Drama"}]}`))
	}))
	defer server.Close()
	engine := nativetmdb.Engine{DB: f.pool, Connections: t3TMDBConnections{tmdb.Client{BaseURL: server.URL, HTTP: server.Client(), Language: "zh-CN"}}}
	detailID := f.text(t, `SELECT id::text FROM native_tmdb_details WHERE tmdb_id=934001`)
	payload, _ := json.Marshal(map[string]string{"detailId": detailID})
	if e = engine.Process(f.ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}

	// Selective search, original-title search, sorting and filters run before LIMIT.
	q := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, SearchTerm: "Manual title", Limit: 1, Fields: []string{"Overview"}, Years: []string{"2026"}, Genres: []string{"custom"}, SortBy: []string{"SortName"}}
	items, total := f.query(t, q)
	if total != 1 || len(items) != 1 {
		t.Fatal(total, items)
	}
	hints, e := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{SearchTerm: "Manual Original", Limit: 10}})
	if e != nil || hints.Total != 1 {
		t.Fatal("effective original-title hint", hints, e)
	}
	item := items[0]
	if item.Name != "Manual title" || item.Overview != "Manual overview" || item.SortName != "A manual sort" || item.OriginalTitle != "Manual Original" || item.CommunityRating == nil || *item.CommunityRating != 6.7 || len(item.Tags) != 1 || len(item.Taglines) != 1 || len(item.MediaSources) != 2 {
		t.Fatal(item)
	}
	fast := q
	fast.SearchTerm = ""
	fi, n := f.query(t, fast)
	fast.IncludeItemTypes = []string{"Movie", "Movie"}
	gi, gn := f.query(t, fast)
	if n != gn || !reflect.DeepEqual(fi, gi) {
		t.Fatal("fast/generic effective mismatch", fi, gi)
	}
	catalog := managed.Engine{DB: f.pool}
	page, e := catalog.ListClassified(f.ctx, "Manual title", "", "", "", 1, 1)
	if e != nil || page.Total != 2 || page.Items[0].Title != item.Name {
		t.Fatal(page, e)
	}
	detail, e := catalog.ManagedDetail(f.ctx, a.asset)
	if e != nil || detail.Title != item.Name || detail.Overview != item.Overview || detail.Category != "电影 / 华语电影" {
		t.Fatal(detail, e)
	}
	// Only one editor with the same revision can win; audits are atomic.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"overview": fmt.Sprint("Edit", i)})})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, metadataedit.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	r, e = repo.Read(f.ctx, a.asset)
	if e != nil {
		t.Fatal(e)
	}
	_, e = repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Restore: metadataedit.Fields})
	if e != nil {
		t.Fatal(e)
	}
	r, e = repo.Read(f.ctx, a.asset)
	if e != nil || len(r.Overrides) != 0 || r.Revision != 3 || string(r.Effective["overview"]) != `"Newest scrape"` {
		t.Fatal(r, e)
	}
	// Clearing is not restoring; provider hydration cannot revive empty fields.
	r, e = repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"overview": "", "genres": []string{}, "communityRating": nil})})
	if e != nil {
		t.Fatal(e)
	}
	empty, _ := f.query(t, nativelibrary.ItemQuery{IDs: []string{item.Id}, Fields: []string{"Overview"}, Limit: 1})
	if empty[0].Overview != "" || len(empty[0].Genres) != 0 || empty[0].CommunityRating != nil {
		t.Fatal(empty[0])
	}
	var audits int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_logs WHERE action='media.metadata.save'`).Scan(&audits); e != nil {
		t.Fatal(e)
	}
	if audits != 4 {
		t.Fatal(audits)
	}
}
func TestT3NumberedHierarchyAndRetainedUnmatched(t *testing.T) {
	f := newBrowseFixture(t)
	f.library(t, "g:t3-tv", "tvshows", true, false)
	m := f.media(t, 934002, "Series", "2025", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	a := f.episode(t, m, "g:t3-tv", 1, 1, 2)
	repo := metadataedit.Repository{DB: f.pool}
	for _, id := range []string{"series_" + m, "season_" + m + "_1", "episode_" + m + "_1_1", "episode_" + m + "_1_2"} {
		r, e := repo.Read(f.ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		_, e = repo.Save(f.ctx, id, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"title": "Manual " + r.Target.Kind, "overview": "Own " + r.Target.Kind, "communityRating": 8.4})})
		if e != nil {
			t.Fatal(e)
		}
	}
	r, e := repo.Read(f.ctx, a.asset)
	if e != nil || len(r.Choices) != 2 {
		t.Fatal(r, e)
	}
	items, _ := f.query(t, nativelibrary.ItemQuery{IncludeItemTypes: []string{"Series", "Season", "Episode"}, Recursive: boolPtr(true), Fields: []string{"Overview"}, Limit: 30})
	for _, x := range items {
		if x.Name != "Manual "+stringLower(string(x.Type)) || x.Overview != "Own "+stringLower(string(x.Type)) {
			t.Fatal(x)
		}
	}
	f.exec(t, `UPDATE media_asset_bindings SET season=2,episode=3,episodes='[3]' WHERE asset_id=$1`, a.asset)
	old, e := repo.Read(f.ctx, "episode_"+m+"_1_1")
	if e != nil || old.Matched || len(old.Unmatched) != 3 {
		t.Fatal(old, e)
	}
	now, e := repo.Read(f.ctx, a.asset)
	if e != nil || len(now.Overrides) != 0 || !now.Matched {
		t.Fatal(now, e)
	}
	// An old asset form cannot silently write to corrected season/episode identity.
	_, e = repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"title": "Wrong"})})
	if !errors.Is(e, metadataedit.ErrConflict) {
		t.Fatal(e)
	}
}
func stringLower(s string) string {
	if s == "Series" {
		return "series"
	}
	if s == "Season" {
		return "season"
	}
	return "episode"
}

type t3TMDBConnections struct{ Client tmdb.Client }

func (c t3TMDBConnections) TMDB(context.Context) (tmdb.Client, error) { return c.Client, nil }
func TestT3OverlayCommitInvalidatesNativeCache(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 934008, "Cache Original", "2025", `{"officialRating":"G"}`)
	a := f.asset(t, m, "g:test-a", time.Now(), "mkv")
	cache := nativecatalog.NewCachedBrowse(f.repo, nativeCacheRedis(t))
	q := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 1}
	old, _, e := cache.Query(f.ctx, f.user, f.permissions(), q)
	if e != nil || old[0].Name != "Cache Original" {
		t.Fatal(old, e)
	}
	repo := metadataedit.Repository{DB: f.pool}
	r, e := repo.Read(f.ctx, a.asset)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"title": "Cache Changed"})}); e != nil {
		t.Fatal(e)
	}
	updated, _, e := cache.Query(f.ctx, f.user, f.permissions(), q)
	if e != nil || updated[0].Name != "Cache Changed" {
		t.Fatal(updated, e)
	}
}

func TestT3UnmatchedMetadataPagination(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 934011, "Unmatched", "2025", `{}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	f.exec(t, `INSERT INTO media_metadata_overrides(media_id,entity_type,season_number,episode_number,overrides)SELECT $1,'episode',1,i,'{"title":"Retained"}'::jsonb FROM generate_series(1,51)i`, m)
	repo := metadataedit.Repository{DB: f.pool}
	first, e := repo.ReadPage(f.ctx, "series_"+m, 1)
	if e != nil || len(first.Unmatched) != 50 || first.UnmatchedTotal != 51 {
		t.Fatal(first, e)
	}
	last, e := repo.ReadPage(f.ctx, "series_"+m, 2)
	if e != nil || len(last.Unmatched) != 1 || last.Unmatched[0].Episode != 51 {
		t.Fatal(last, e)
	}
}

func TestT3EffectiveRatingIsMetadataNotUserRestriction(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 934012, "Rating", "2025", `{"officialRating":"G"}`)
	a := f.asset(t, m, "g:test-a", time.Now(), "mkv")
	q := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 1}
	items, total := f.query(t, q)
	if len(items) != 1 || total != 1 {
		t.Fatal(items, total)
	}
	repo := metadataedit.Repository{DB: f.pool}
	r, e := repo.Read(f.ctx, a.asset)
	if e != nil {
		t.Fatal(e)
	}
	_, e = repo.Save(f.ctx, a.asset, "editor", metadataedit.Patch{TargetKey: r.TargetKey, Revision: r.Revision, Set: t3Values(map[string]any{"officialRating": "R"})})
	if e != nil {
		t.Fatal(e)
	}
	items, total = f.query(t, q)
	if len(items) != 1 || total != 1 || items[0].OfficialRating != "R" {
		t.Fatal("effective rating changed visibility or metadata", items, total)
	}
}
