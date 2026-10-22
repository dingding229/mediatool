package migrations_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type movieFixture struct {
	nativeFixture
	pool    *pgxpool.Pool
	storage string
	parents map[string]string
}
type movieAsset struct{ asset, file string }

func newMovieFixture(t *testing.T) movieFixture {
	t.Helper()
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 58); err != nil {
		t.Fatal(err)
	}
	fixture := movieFixture{nativeFixture: f, pool: catalogTestPool(t, f), storage: jobs.UUID(), parents: map[string]string{}}
	f.exec(t, `INSERT INTO storage_connections(id,name,provider) VALUES($1,'Synthetic catalog storage','clouddrive2')`, fixture.storage)
	for _, id := range []string{"g:test-a", "g:test-b"} {
		ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: fixture.pool}, []nativelibrary.Key{{EntityType: nativelibrary.EntityLibrary, InternalKey: id}})
		if err != nil {
			t.Fatal(err)
		}
		itemID := ids[nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: id}]
		fixture.parents[id] = strconv.FormatInt(itemID, 10)
		f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type) VALUES($1,$2,$3,$3,$3,'movies')`, id, itemID, strings.TrimPrefix(id, "g:"))
	}
	return fixture
}
func (f movieFixture) media(t *testing.T, tmdb int64, title, year string, meta string) string {
	t.Helper()
	id := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata) VALUES($1,'movie',$2,$3,$4,$5)`, id, tmdb, title, year, json.RawMessage(meta))
	return id
}
func (f movieFixture) asset(t *testing.T, media, lib string, created time.Time, extension string) movieAsset {
	t.Helper()
	asset, file := jobs.UUID(), jobs.UUID()
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state) VALUES($1,$2,$3,1,'present')`, file, f.storage, "/115/synthetic/"+file+"."+extension)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,created_at) VALUES($1,$2,$3)`, asset, file, created)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state) VALUES($1,$2,'ready')`, asset, media)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference) VALUES($1,$2,$3,'synthetic-only')`, jobs.UUID(), asset, "Synthetic/"+asset+".strm")
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision) VALUES($1,$2,'output_directory',1,1)`, asset, lib)
	return movieAsset{asset, file}
}
func (f movieFixture) query(t *testing.T, q catalog.MovieQuery) ([]dto.BaseItemDto, int) {
	t.Helper()
	items, total, err := (catalog.MovieRepository{DB: f.pool}).Query(f.ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	return items, total
}
func (f movieFixture) lifecycle(t *testing.T) {
	t.Helper()
	cursor := int64(0)
	for i := 0; i < 10; i++ {
		next, count, err := catalog.RefreshIDLifecycle(f.ctx, f.pool, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if count < 500 {
			return
		}
		cursor = next
	}
	t.Fatal("lifecycle did not finish")
}
func TestNativeMovieProjectionAggregatesOnlyExposableVersionsAndLibraryParent(t *testing.T) {
	f := newMovieFixture(t)
	stamp := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	media := f.media(t, 900001, "Synthetic Film", "2026", `{"originalTitle":"Synthetic Original","overview":"Synthetic overview","posterPath":"/synthetic-poster.jpg","backdropPath":"/synthetic-backdrop.jpg","genres":["Drama"],"premiereDate":"2026-01-02","communityRating":8.5,"officialRating":"PG","imdbId":"tt900001"}`)
	a := f.asset(t, media, "g:test-a", stamp.Add(time.Hour), "mkv")
	b := f.asset(t, media, "g:test-a", stamp, "mp4")
	other := f.asset(t, media, "g:test-b", stamp.Add(2*time.Hour), "mkv")
	missing := f.asset(t, media, "g:test-a", stamp.Add(-time.Hour), "mkv")
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, missing.file)
	// Multiple STRM outputs must not multiply either the movie or its media sources/counts.
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference) VALUES($1,$2,'Synthetic/duplicate.strm','synthetic-only')`, jobs.UUID(), a.asset)
	if n := f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie'`); n != 0 {
		t.Fatal("ID allocated without query", n)
	}
	items, total := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if total != 1 || len(items) != 1 || len(items[0].MediaSources) != 2 {
		t.Fatal(total, items)
	}
	item := items[0]
	if item.Type != dto.Movie || item.IsFolder || item.ParentId != f.parents["g:test-a"] || item.DateCreated == nil || !item.DateCreated.Time().Equal(stamp) || item.UserData == nil {
		t.Fatalf("bad movie projection: %+v", item)
	}
	if item.OriginalTitle != "Synthetic Original" || item.ProviderIds["Tmdb"] != "900001" || item.ProviderIds["Imdb"] != "tt900001" || item.OfficialRating != "PG" || item.ImageTags["Primary"] != nativelibrary.ImageTag("/synthetic-poster.jpg") {
		t.Fatal(item)
	}
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at) VALUES('movie',900001,'zh-CN',$1,'ready',now())`, json.RawMessage(`{"premiereDate":"2026-02-03","communityRating":9.1,"officialRating":"R","imdbId":"tt900001","runtimeMinutes":123,"people":[{"id":10,"name":"TMDB Actor","type":"Actor","role":"Lead","imagePath":"/actor.jpg"}]}`))
	enriched, _ := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if enriched[0].PremiereDate == nil || enriched[0].PremiereDate.Time().Format("2006-01-02") != "2026-02-03" || enriched[0].CommunityRating == nil || *enriched[0].CommunityRating != 9.1 || enriched[0].OfficialRating != "R" || enriched[0].RunTimeTicks == nil || len(enriched[0].People) != 1 {
		t.Fatal("TMDB snapshot not projected", enriched[0])
	}
	seen := map[string]bool{}
	for _, source := range item.MediaSources {
		seen[source.Id] = true
		if source.Path != "" || source.SupportsTranscoding() {
			t.Fatal(source)
		}
	}
	if !seen[a.asset] || !seen[b.asset] || seen[other.asset] || seen[missing.asset] {
		t.Fatal(seen)
	}
	second, _ := f.query(t, catalog.MovieQuery{LibraryID: "g:test-b", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-b", AllowedIDs: []string{"g:test-b"}}, Limit: 200})
	if len(second) != 1 || second[0].Id != item.Id || second[0].ParentId != f.parents["g:test-b"] || len(second[0].MediaSources) != 1 {
		t.Fatal(second)
	}
	repeated, _ := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if repeated[0].Id != item.Id || f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie'`) != 1 {
		t.Fatal("movie ID not stable")
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"/115/", "Synthetic/duplicate.strm", "key_reference", "pickCode", "strm302"} {
		if strings.Contains(string(raw), sensitive) {
			t.Fatal("private value leaked")
		}
	}
	page, err := (catalog.AssignmentRepository{DB: f.pool}).ListAssets(f.ctx, "g:test-a", 0, 200)
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatal("shared visibility/count drift", page, err)
	}
	libraries, err := (catalog.Repository{DB: f.pool}).List(f.ctx, 0, 200)
	if err != nil {
		t.Fatal("list fallback and classified libraries", err)
	}
	found := false
	for _, lib := range libraries.Items {
		if lib.ID == "g:test-a" {
			found = true
			if lib.AssetCount != 2 {
				t.Fatal("duplicate STRM counted twice", lib.AssetCount)
			}
		}
	}
	if !found {
		t.Fatal("classified library missing")
	}
	// Invalid variants and other media types do not create native movie IDs.
	for index, state := range []string{"pending", "missing", "recycled", "no-strm", "invalid-strm", "unassigned", "series"} {
		mid := f.media(t, 900010+int64(index), "Excluded "+state, "2026", `{}`)
		x := f.asset(t, mid, "g:test-a", stamp, "mkv")
		switch state {
		case "pending":
			f.exec(t, `UPDATE media_asset_bindings SET state='pending' WHERE asset_id=$1`, x.asset)
		case "missing", "recycled":
			f.exec(t, `UPDATE cloud_files SET state=$2 WHERE id=$1`, x.file, state)
		case "no-strm":
			f.exec(t, `DELETE FROM strm_entries WHERE asset_id=$1`, x.asset)
		case "invalid-strm":
			f.exec(t, `UPDATE strm_entries SET output_path=$2 WHERE asset_id=$1`, x.asset, "Synthetic/"+x.asset+".txt")
		case "unassigned":
			f.exec(t, `DELETE FROM native_asset_libraries WHERE asset_id=$1`, x.asset)
		case "series":
			f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, mid)
		}
	}
	filtered, n := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if n != 1 || len(filtered) != 1 || f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie'`) != 1 {
		t.Fatal(n, filtered)
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	hidden, n := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if hidden == nil || len(hidden) != 0 || n != 0 {
		t.Fatal(hidden, n)
	}
}
func TestNativeMoviePaginationSortingAndLimits(t *testing.T) {
	f := newMovieFixture(t)
	stamp := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	for _, x := range []struct {
		id                   int64
		name, year, metadata string
		hour                 int
	}{
		{910001, "Zebra", "2024", `{"sortName":"Alpha","communityRating":7}`, 2},
		{910002, "Alpha", "2026", `{"sortName":"Beta","communityRating":9}`, 1},
		{910003, "Beta", "2025", `{"sortName":"Beta","communityRating":8}`, 3},
	} {
		mid := f.media(t, x.id, x.name, x.year, x.metadata)
		f.asset(t, mid, "g:test-a", stamp.Add(time.Duration(x.hour)*time.Hour), "mkv")
	}
	for _, test := range []struct {
		sort       string
		descending bool
		want       []string
	}{
		{"SortName", false, []string{"910001", "910002", "910003"}}, {"SortName", true, []string{"910002", "910003", "910001"}},
		{"DateCreated", false, []string{"910002", "910001", "910003"}}, {"DateCreated", true, []string{"910003", "910001", "910002"}},
		{"ProductionYear", false, []string{"910001", "910003", "910002"}}, {"ProductionYear", true, []string{"910002", "910003", "910001"}},
		{"CommunityRating", false, []string{"910001", "910003", "910002"}}, {"CommunityRating", true, []string{"910002", "910003", "910001"}},
	} {
		t.Run(test.sort+strconv.FormatBool(test.descending), func(t *testing.T) {
			q := catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 2, SortBy: test.sort, Descending: test.descending}
			first, n := f.query(t, q)
			if n != 3 || len(first) != 2 {
				t.Fatal(first, n)
			}
			q.StartIndex = 2
			last, n := f.query(t, q)
			if n != 3 || len(last) != 1 {
				t.Fatal(last, n)
			}
			got := []string{first[0].ProviderIds["Tmdb"], first[1].ProviderIds["Tmdb"], last[0].ProviderIds["Tmdb"]}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatal(got, test.want)
			}
			q.StartIndex = 3
			empty, n := f.query(t, q)
			if n != 3 || len(empty) != 0 || empty == nil {
				t.Fatal(empty, n)
			}
		})
	}
	for _, q := range []catalog.MovieQuery{{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 201}, {LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 0}, {LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1, StartIndex: -1}, {LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1, StartIndex: 1000001}, {LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1, SortBy: "title;DROP TABLE media_items"}, {Limit: 1}} {
		if _, _, err := (catalog.MovieRepository{DB: f.pool}).Query(f.ctx, q); err == nil {
			t.Fatal("invalid query accepted", q)
		}
	}
}
func TestNativeIDLifecycleRetiresOnlyWithoutAnyExposableVersionAndRestores(t *testing.T) {
	f := newMovieFixture(t)
	stamp := time.Now().UTC()
	mid := f.media(t, 920001, "Synthetic lifecycle", "2026", `{}`)
	a := f.asset(t, mid, "g:test-a", stamp, "mkv")
	b := f.asset(t, mid, "g:test-b", stamp, "mkv")
	items, _ := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1})
	id := items[0].Id
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE id=$1 AND retired_at IS NULL`, id) != 1 {
		t.Fatal("live movie retired")
	}
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, a.file)
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE id=$1 AND retired_at IS NULL`, id) != 1 {
		t.Fatal("another library has a live version")
	}
	f.exec(t, `DELETE FROM strm_entries WHERE asset_id=$1`, b.asset)
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE id=$1 AND retired_at IS NOT NULL`, id) != 1 {
		t.Fatal("unexposable movie not retired")
	}
	firstRetired := f.text(t, `SELECT retired_at::text FROM native_item_ids WHERE id=$1`, id)
	f.lifecycle(t)
	if f.text(t, `SELECT retired_at::text FROM native_item_ids WHERE id=$1`, id) != firstRetired {
		t.Fatal("retirement not idempotent")
	}
	f.exec(t, `UPDATE cloud_files SET state='present' WHERE id=$1`, a.file)
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE id=$1 AND retired_at IS NULL`, id) != 1 {
		t.Fatal("reappearing movie not restored")
	}
	items, _ = f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1})
	if items[0].Id != id {
		t.Fatal("restored ID changed")
	}
	// Allocate enough synthetic absent keys to verify page bound and restart-safe continuation.
	for base := int64(0); base < 502; base += 500 {
		keys := []nativelibrary.Key{}
		for i := base; i < base+500 && i < 502; i++ {
			key, err := nativelibrary.MovieKey(930000 + i)
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, key)
		}
		if _, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, keys); err != nil {
			t.Fatal(err)
		}
	}
	next, n, err := catalog.RefreshIDLifecycle(f.ctx, f.pool, 0)
	if err != nil || n != 500 {
		t.Fatal(next, n, err)
	}
	_, n, err = catalog.RefreshIDLifecycle(f.ctx, f.pool, next)
	if err != nil || n != 3 {
		t.Fatal(n, err)
	}
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie' AND retired_at IS NOT NULL`) != 502 {
		t.Fatal("lifecycle did not converge")
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='library' AND retired_at IS NOT NULL`) != 0 {
		t.Fatal("library IDs retired")
	}
}

func TestNativeHierarchyIDLifecycleUsesFirstEpisodeAndRetainsMappings(t *testing.T) {
	f := newMovieFixture(t)
	mid := f.media(t, 940001, "Synthetic Series", "2026", `{}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, mid)
	asset := f.asset(t, mid, "g:test-a", time.Now().UTC(), "mkv")
	f.exec(t, `UPDATE media_asset_bindings SET season=1,episode=1,episodes='[1,2]' WHERE asset_id=$1`, asset.asset)
	series, err := nativelibrary.SeriesKey(940001)
	if err != nil {
		t.Fatal(err)
	}
	season, err := nativelibrary.SeasonKey(940001, 1)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := nativelibrary.EpisodeKey(940001, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{series, season, episode})
	if err != nil {
		t.Fatal(err)
	}
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type<>'library' AND retired_at IS NULL`) != 3 {
		t.Fatal("live hierarchy retired")
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE internal_key='tmdb:940001:s1e2'`) != 0 {
		t.Fatal("multi-episode file allocated a duplicate item")
	}
	f.exec(t, `UPDATE media_asset_bindings SET state='pending' WHERE asset_id=$1`, asset.asset)
	f.lifecycle(t)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type<>'library' AND retired_at IS NOT NULL`) != 3 {
		t.Fatal("unconfirmed hierarchy not retired")
	}
	f.exec(t, `UPDATE media_asset_bindings SET state='ready' WHERE asset_id=$1`, asset.asset)
	f.lifecycle(t)
	restored, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{series, season, episode})
	if err != nil || !reflect.DeepEqual(ids, restored) {
		t.Fatal(ids, restored, err)
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type<>'library' AND retired_at IS NULL`) != 3 {
		t.Fatal("hierarchy not restored")
	}
}
func TestNativeMovieSourceLimitIsExplicitAndMissingMetadataStaysAbsent(t *testing.T) {
	f := newMovieFixture(t)
	mid := f.media(t, 950001, "Synthetic Sparse Film", "", `null`)
	for i := 0; i < 100; i++ {
		f.asset(t, mid, "g:test-a", time.Date(2026, 10, 16, 0, 0, i, 0, time.UTC), "mkv")
	}
	items, n := f.query(t, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1})
	if n != 1 || len(items) != 1 || len(items[0].MediaSources) != 100 {
		t.Fatal(n, len(items))
	}
	item := items[0]
	if item.ProductionYear != nil || item.CommunityRating != nil || item.OfficialRating != "" || item.RunTimeTicks != nil || item.PremiereDate != nil || len(item.ImageTags) != 0 || item.OriginalTitle != "" {
		t.Fatal("missing metadata was fabricated", item)
	}
	f.asset(t, mid, "g:test-a", time.Now().UTC(), "mkv")
	if _, _, err := (catalog.MovieRepository{DB: f.pool}).Query(f.ctx, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 1}); err == nil {
		t.Fatal("unbounded sources accepted")
	}
}
