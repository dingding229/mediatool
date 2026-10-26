package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/mediausers/nativepermissions"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type browseTrace struct {
	t                           *testing.T
	queries, snapshots, sources atomic.Int64
	idWrites                    atomic.Int64
	probes                      atomic.Int64
	stills                      atomic.Int64
	phaseMu                     sync.Mutex
	timings                     map[string]time.Duration
}

type browsePhaseKey struct{}
type browsePhase struct {
	name  string
	start time.Time
}

func (tr *browseTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tr.queries.Add(1)
	if strings.Contains(data.SQL, "/* native.still.page */") {
		tr.stills.Add(1)
	}
	if strings.Contains(data.SQL, "/* native.probe.page */") {
		tr.probes.Add(1)
	}
	if strings.Contains(data.SQL, "INSERT INTO native_item_ids") || strings.Contains(data.SQL, "UPDATE native_item_ids") {
		tr.idWrites.Add(1)
	}
	if strings.Contains(data.SQL, "SELECT w.kind,w.tmdb,w.s") {
		tr.snapshots.Add(1)
	}
	if strings.Contains(data.SQL, "max(versions)>100") {
		tr.sources.Add(1)
	}
	phase := "other"
	switch {
	case strings.Contains(data.SQL, "/* native.still.page */"):
		phase = "stills"
	case strings.Contains(data.SQL, "/* native.probe.page */"):
		phase = "probes"
	case strings.Contains(data.SQL, "LIMIT 500"):
		phase = "id-scan"
	case strings.HasPrefix(data.SQL, "INSERT INTO native_item_ids"):
		phase = "id-insert"
	case strings.Contains(data.SQL, "filtered AS"):
		phase = "page"
	case strings.Contains(data.SQL, "max(versions)>100"):
		phase = "sources"
	case strings.Contains(data.SQL, "SELECT w.kind,w.tmdb,w.s"):
		phase = "snapshots"
	}
	return context.WithValue(ctx, browsePhaseKey{}, browsePhase{phase, time.Now()})
}
func (tr *browseTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	phase, _ := ctx.Value(browsePhaseKey{}).(browsePhase)
	tr.phaseMu.Lock()
	if tr.timings == nil {
		tr.timings = map[string]time.Duration{}
	}
	tr.timings[phase.name] += time.Since(phase.start)
	tr.phaseMu.Unlock()
	if data.Err != nil {
		tr.t.Logf("synthetic browse SQL phase=%s elapsed=%s err=%v", phase.name, time.Since(phase.start), data.Err)
	}
}

type browseFixture struct {
	movieFixture
	user  contracts.User
	repo  catalog.BrowseRepository
	trace *browseTrace
}

func newBrowseFixture(t *testing.T, versions ...int64) browseFixture {
	target := int64(84)
	if len(versions) > 0 {
		target = versions[0]
	}
	t.Helper()
	f := newMovieFixture(t)
	if _, err := f.provider.UpTo(f.ctx, target); err != nil {
		t.Fatal(err)
	}
	config := f.pool.Config()
	trace := &browseTrace{t: t}
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f.pool = pool
	account, err := (mediausers.Repository{DB: pool}).Save(f.ctx, "", mediausers.Input{Username: "browse.member", Password: "Browse123", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	login, err := (&nativeauth.Store{DB: pool, Secret: "synthetic-browse-key"}).Login(f.ctx, account.Username, "Browse123", contracts.DeviceInfo{DeviceID: "browse-device"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"g:test-a", "g:test-b"} {
		f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,$2,true)`, account.ID, id)
	}
	return browseFixture{f, login.User, catalog.BrowseRepository{DB: pool}, trace}
}
func (f browseFixture) permissions() *nativepermissions.Checker { return nativepermissions.New(f.pool) }
func (f browseFixture) query(t *testing.T, q nativelibrary.ItemQuery) ([]dto.BaseItemDto, int) {
	t.Helper()
	items, total, err := f.repo.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil {
		t.Fatal("browse query", err)
	}
	if items == nil {
		t.Fatal("nil items")
	}
	return items, total
}
func (f browseFixture) library(t *testing.T, id, collection string, grant, hidden bool) string {
	t.Helper()
	key := nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: id}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{key})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type,hidden)VALUES($1,$2,$3,$3,$3,$4,$5)`, id, ids[key], strings.TrimPrefix(id, "g:"), collection, hidden)
	if grant {
		f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1::uuid,$2,true)`, f.user.ID, id)
	}
	return strconv.FormatInt(ids[key], 10)
}
func (f browseFixture) episode(t *testing.T, media, lib string, season, first, last int) movieAsset {
	t.Helper()
	asset := f.asset(t, media, lib, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "mkv")
	episodes := []int{}
	for n := first; n <= last; n++ {
		episodes = append(episodes, n)
	}
	raw, err := json.Marshal(episodes)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE media_asset_bindings SET season=$2,episode=$3,episodes=$4 WHERE asset_id=$1`, asset.asset, season, first, raw)
	identity, err := json.Marshal(map[string]any{"episodeDetails": []map[string]any{{"season": season, "number": first, "title": fmt.Sprintf("Synthetic Episode %d", first), "airDate": "2025-01-01", "runtimeMinutes": 24}}})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE media_assets SET media_identity=$2 WHERE id=$1`, asset.asset, identity)
	return asset
}
func boolPtr(v bool) *bool { return &v }
func TestNativeBrowseCrossLibraryHierarchyAndMixedIDs(t *testing.T) {
	f := newBrowseFixture(t)
	now := time.Now()
	m := f.media(t, 981001, "Synthetic Shared Movie", "2025", `{"officialRating":"G","genres":["Drama"]}`)
	a := f.asset(t, m, "g:test-a", now, "mkv")
	b := f.asset(t, m, "g:test-b", now, "mp4")
	tvA := f.library(t, "g:tv-a", "tvshows", true, false)
	f.library(t, "g:tv-b", "tvshows", true, false)
	hidden := f.library(t, "g:hidden", "movies", true, true)
	private := f.library(t, "g:private", "movies", false, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'series',981002,'Synthetic Shared Show','2025','{"officialRating":"PG-13","genres":["Drama"]}')`, show)
	f.episode(t, show, "g:tv-a", 0, 1, 1)
	f.episode(t, show, "g:tv-b", 0, 1, 1)
	f.episode(t, show, "g:tv-a", 1, 1, 1)
	f.episode(t, show, "g:tv-b", 1, 2, 2)
	f.episode(t, show, "g:tv-a", 1, 3, 4)
	q := nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie", "Series", "Episode"}}
	items, total := f.query(t, q)
	if total != 6 || len(items) != 6 {
		t.Fatal("global dedup", len(items), total)
	}
	var movieID, seriesID, specialID string
	for _, v := range items {
		switch v.Type {
		case dto.Movie:
			movieID = v.Id
			if len(v.MediaSources) != 2 {
				t.Fatal("global sources", v.MediaSources)
			}
		case dto.Series:
			seriesID = v.Id
			if *v.ChildCount != 2 || *v.RecursiveItemCount != 4 {
				t.Fatal("series counts", v)
			}
		case dto.Episode:
			if *v.ParentIndexNumber == 0 {
				specialID = v.Id
				if len(v.MediaSources) != 2 {
					t.Fatal("special editions")
				}
			}
		}
	}
	scoped, n := f.query(t, nativelibrary.ItemQuery{ParentID: f.parents["g:test-a"], Limit: 200})
	if n != 1 || len(scoped[0].MediaSources) != 1 || scoped[0].MediaSources[0].Id != a.asset {
		t.Fatal("library A borrowed editions", scoped)
	}
	scoped, n = f.query(t, nativelibrary.ItemQuery{ParentID: f.parents["g:test-b"], Limit: 200})
	if n != 1 || len(scoped[0].MediaSources) != 1 || scoped[0].MediaSources[0].Id != b.asset || scoped[0].Id != movieID {
		t.Fatal("library B borrowed editions")
	}
	eps, n := f.query(t, nativelibrary.ItemQuery{ParentID: tvA, Recursive: boolPtr(true), IncludeItemTypes: []string{"Episode"}, Limit: 200})
	if n != 3 {
		t.Fatal("library episodes", n)
	}
	for _, v := range eps {
		if *v.ParentIndexNumber == 1 && *v.IndexNumber == 2 {
			t.Fatal("foreign-library episode leaked")
		}
		if v.Id == specialID && len(v.MediaSources) != 1 {
			t.Fatal("foreign edition leaked")
		}
		if *v.IndexNumber == 3 && (v.IndexNumberEnd == nil || *v.IndexNumberEnd != 4) {
			t.Fatal("multi-episode mapping")
		}
	}
	seasons, n := f.query(t, nativelibrary.ItemQuery{ParentID: seriesID, Limit: 200, SortBy: []string{"IndexNumber"}})
	if n != 2 || *seasons[0].IndexNumber != 0 {
		t.Fatal("seasons", seasons, n)
	}
	episodes, n := f.query(t, nativelibrary.ItemQuery{ParentID: seasons[1].Id, Limit: 200, SortBy: []string{"IndexNumber"}})
	if n != 3 || *episodes[2].IndexNumber != 3 {
		t.Fatal("season episodes", episodes, n)
	}
	before, snapshots, sources, probes := f.trace.queries.Load(), f.trace.snapshots.Load(), f.trace.sources.Load(), f.trace.probes.Load()
	batch, n := f.query(t, nativelibrary.ItemQuery{IDs: []string{movieID, seriesID, seasons[0].Id, specialID, tvA, movieID, hidden, private, "99999999999"}, Limit: 200})
	if n != 5 || len(batch) != 5 {
		t.Fatal("mixed ID batch", len(batch), n)
	}
	if f.trace.queries.Load()-before > 12 || f.trace.snapshots.Load()-snapshots != 1 || f.trace.sources.Load()-sources != 1 || f.trace.probes.Load()-probes != 1 {
		t.Fatal("N+1 batch hydration", f.trace.queries.Load()-before)
	}
	for _, id := range []string{hidden, private, "9999999999"} {
		_, _, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: id, Limit: 200})
		if !errors.Is(err, nativelibrary.ErrNotFound) {
			t.Fatal("parent existence leaked", id, err)
		}
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:tv-a'`)
	_, _, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: tvA, Limit: 200})
	if !errors.Is(err, nativelibrary.ErrNotFound) {
		t.Fatal("hidden parent", err)
	}
	counts, err := f.repo.Counts(f.ctx, f.user, f.permissions())
	if err != nil || counts.MovieCount != 1 || counts.SeriesCount != 1 || counts.EpisodeCount != 2 {
		t.Fatal("visible counts", counts, err)
	}
	views, err := f.repo.Views(f.ctx, f.user, f.permissions())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Id == hidden || v.Id == private || v.Id == tvA {
			t.Fatal("hidden/private view")
		}
	}
	f.exec(t, `UPDATE media_items SET metadata=jsonb_set(metadata,'{officialRating}','"R"') WHERE id=$1`, show)
	f.exec(t, `UPDATE media_users SET max_parental_rating=65 WHERE id=$1::uuid`, f.user.ID)
	_, _, err = f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: seriesID, Limit: 200})
	if !errors.Is(err, nativelibrary.ErrNotFound) {
		t.Fatal("restricted series parent", err)
	}
	restricted, n := f.query(t, nativelibrary.ItemQuery{IDs: []string{seriesID, seasons[1].Id, specialID}, Limit: 200})
	if len(restricted) != 0 || n != 0 {
		t.Fatal("restricted hierarchy leaked")
	}

}

func TestNativeBrowseParentalSQLFiltersAndExactPagination(t *testing.T) {
	f := newBrowseFixture(t)
	now := time.Now()
	for i, rating := range []string{"G", "R", "PG-13", "", "G", "PG", "G"} {
		genres, year := "Drama", "2025"
		if i == 4 {
			genres = "Action"
			year = "2024"
		}
		m := f.media(t, 982000+int64(i), fmt.Sprintf("Synthetic Movie %d", i), year, fmt.Sprintf(`{"officialRating":%q,"genres":[%q],"sortName":"Same","communityRating":7.5,"premiereDate":"2025-01-01"}`, rating, genres))
		f.asset(t, m, "g:test-a", now, "mkv")
	}
	// Persisted TMDB regional rating wins over stored show metadata.
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES('movie',982001,'zh-CN','{"officialRating":"US:PG-13","genres":["Drama"]}','ready',now())`)
	f.exec(t, `UPDATE media_users SET max_parental_rating=65 WHERE id=$1::uuid`, f.user.ID)
	q := nativelibrary.ItemQuery{Limit: 2, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Genres: []string{"drama"}, Years: []string{"2025"}, SortBy: []string{"SortName"}}
	ids := []string{}
	for _, offset := range []int64{0, 2, 4, 6} {
		q.StartIndex = offset
		page, total := f.query(t, q)
		if total != 5 {
			t.Fatal("filter count differs from page", total)
		}
		want := min(2, max(0, 5-int(offset)))
		if len(page) != want {
			t.Fatal("last page", offset, len(page), want)
		}
		for _, v := range page {
			ids = append(ids, v.Id)
			if v.OfficialRating == "" || v.OfficialRating == "R" {
				t.Fatal("rating leaked")
			}
		}
	}
	for i := 1; i < len(ids); i++ {
		a := browseNumericID(t, ids[i-1])
		b := browseNumericID(t, ids[i])
		if a >= b {
			t.Fatal("tie must use numeric ID", ids)
		}
	}
	q.StartIndex = 2
	first, _ := f.query(t, q)
	again, _ := f.query(t, q)
	if !reflect.DeepEqual(first, again) {
		t.Fatal("unstable repeated page")
	}
	q.StartIndex = 0
	q.Limit = 200
	q.SearchTerm = "Movie 2"
	page, total := f.query(t, q)
	if total != 1 || len(page) != 1 {
		t.Fatal("title search", total)
	}
	q.SearchTerm = "%"
	_, total = f.query(t, q)
	if total != 0 {
		t.Fatal("search term treated as SQL pattern")
	}
	q.SearchTerm = ""
	q.IsFolder = boolPtr(true)
	_, total = f.query(t, q)
	if total != 0 {
		t.Fatal("folder filter")
	}
	q.IsFolder = nil
	q.IsPlayed = boolPtr(true)
	_, total = f.query(t, q)
	if total != 0 {
		t.Fatal("played before T35")
	}
	q.IsPlayed = nil
	q.SortBy = []string{"Random"}
	q.SortOrder = []string{"Descending"}
	page, total = f.query(t, q)
	if total != 5 || page[0].Id != ids[0] {
		t.Fatal("Random not ignored")
	}
	counts, err := f.repo.Counts(f.ctx, f.user, f.permissions())
	if err != nil || counts.MovieCount != 6 {
		t.Fatal("same parental predicate for counts", counts, err)
	}
	filtered, err := f.repo.Counts(f.ctx, f.user, f.permissions(), q)
	if err != nil || filtered.MovieCount != 5 {
		t.Fatal("counts/query filter drift", filtered, err)
	}
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1::uuid`, f.user.ID)
	denied, n := f.query(t, q)
	if n != 0 || len(denied) != 0 {
		t.Fatal("ungranted user saw movies")
	}
	empty, err := f.repo.Counts(f.ctx, f.user, f.permissions())
	if err != nil || empty != (nativelibrary.ItemCounts{}) {
		t.Fatal("ungranted user counts", empty, err)
	}

}

func TestNativeBrowseViewsFallbackParentAndInvalidAssets(t *testing.T) {
	f := newBrowseFixture(t)
	// Reserved fallback already exists from migration 57; grants are explicit.
	f.exec(t, `UPDATE native_libraries SET hidden=false WHERE id='f:movie'`)
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1::uuid,'f:movie',true)`, f.user.ID)
	var fallback string
	fallback = f.text(t, `SELECT item_id::text FROM native_libraries WHERE id='f:movie'`)
	views, err := f.repo.Views(f.ctx, f.user, f.permissions())
	if err != nil || len(views) != 2 {
		t.Fatal("empty fallback view", views, err)
	}
	_, _, err = f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: fallback, Limit: 200})
	if !errors.Is(err, nativelibrary.ErrNotFound) {
		t.Fatal("empty fallback parent", err)
	}
	m := f.media(t, 983001, "Synthetic fallback", "2025", `{"officialRating":"G"}`)
	asset := f.asset(t, m, "f:movie", time.Now(), "mkv")
	f.exec(t, `UPDATE native_libraries SET sort_order=3 WHERE id='g:test-a'`)
	f.exec(t, `UPDATE native_libraries SET sort_order=1 WHERE id='g:test-b'`)
	views, err = f.repo.Views(f.ctx, f.user, f.permissions())
	if err != nil || len(views) != 3 || views[0].Id != f.parents["g:test-b"] || views[2].Id != fallback {
		t.Fatal("view order", views, err)
	}
	roots, n := f.query(t, nativelibrary.ItemQuery{Limit: 200, IsFolder: boolPtr(false)})
	if len(roots) != 0 || n != 0 {
		t.Fatal("root filter not applied", roots, n)
	}
	_, n = f.query(t, nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 200})
	if n != 1 {
		t.Fatal(n)
	}
	for _, sql := range []string{`UPDATE media_asset_bindings SET state='pending' WHERE asset_id=$1`, `UPDATE media_asset_bindings SET state='ready' WHERE asset_id=$1`, `UPDATE cloud_files SET state='missing' WHERE id=$1`, `UPDATE cloud_files SET state='present' WHERE id=$1`, `DELETE FROM strm_entries WHERE asset_id=$1`} {
		key := asset.asset
		if strings.Contains(sql, "cloud_files") {
			key = asset.file
		}
		f.exec(t, sql, key)
		_, n = f.query(t, nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 200})
		want := 0
		if strings.Contains(sql, "state='ready'") || strings.Contains(sql, "state='present'") {
			want = 1
		}
		if n != want {
			t.Fatal("invalid asset exposed", sql, n)
		}
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	_, _, err = f.repo.Query(ctx, f.user, f.permissions(), nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true)})
	if !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal("cancellation not propagated", err)
	}
}

func seedBrowseMovies(t *testing.T, f browseFixture, count int) {
	t.Helper()
	// All data is generated inside this test's disposable schema in media_tool_test.
	f.exec(t, `CREATE TEMP TABLE native_browse_seed AS SELECT i,md5('browse-movie-'||i)::uuid media,md5('browse-file-'||i)::uuid file,md5('browse-asset-'||i)::uuid asset FROM generate_series(1,$1::int) i`, count)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata) SELECT media,'movie',990000+i,'Synthetic movie '||lpad(i::text,5,'0'),'2025','{"sortName":"Same Sort","genres":["Drama"],"officialRating":"G"}' FROM native_browse_seed`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state) SELECT file,$1,'/115/synthetic/'||i||'.mkv',1,'present' FROM native_browse_seed`, f.storage)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,created_at)SELECT asset,file,'2026-01-01T00:00:00Z'::timestamptz FROM native_browse_seed`)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state)SELECT asset,media,'ready' FROM native_browse_seed`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT md5('browse-strm-'||i)::uuid,asset,'Synthetic/'||i||'.strm','synthetic' FROM native_browse_seed`)
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset,'g:test-a','output_directory',1,1 FROM native_browse_seed`)
	// Simulate background repair, outside the first-browse stopwatch.
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}

	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,state,result,probed_at)
 SELECT f.id,native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),'ready','{"schema_version":1,"durationTicks":600000000,"container":"mkv","size":12345,"bitRate":123456,"streams":[{"index":0,"type":"Video","codec":"h264","width":1920,"height":1080,"videoRange":"SDR","videoRangeType":"SDR"}]}',now() FROM native_browse_seed seed JOIN cloud_files f ON f.id=seed.file`)
}

func TestNativeBrowseTenThousandMoviesBudgetAndBatchBound(t *testing.T) {
	f := newBrowseFixture(t)
	// The endpoint portion exercises current admission, not historical schema 60.
	if _, e := f.provider.UpTo(f.ctx, 82); e != nil {
		t.Fatal(e)
	}
	seedBrowseMovies(t, f, 10000)
	probesBefore := f.trace.probes.Load()
	stillsBefore := f.trace.stills.Load()
	f.trace.phaseMu.Lock()
	f.trace.timings = map[string]time.Duration{}
	f.trace.phaseMu.Unlock()
	q := nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, SortBy: []string{"SortName"}}
	writes := f.trace.idWrites.Load()
	start := time.Now()
	first, total := f.query(t, q)
	cold := time.Since(start)
	f.trace.phaseMu.Lock()
	t.Logf("T26 cold phase timings=%v", f.trace.timings)
	f.trace.phaseMu.Unlock()
	if f.trace.stills.Load() != stillsBefore {
		t.Fatal("movie page executed episode-image SQL")
	}
	if f.trace.probes.Load()-probesBefore != 1 {
		t.Fatal("page probes not loaded exactly once")
	}
	if f.trace.idWrites.Load() != writes {
		t.Fatal("preallocated browse wrote IDs")
	}
	if total != 10000 || len(first) != 200 || cold >= 5*time.Second {
		t.Fatal("10k cold query budget", total, len(first), cold)
	}
	if len(first[0].MediaStreams) != 1 || first[0].Container != "mkv" || first[0].Width == nil || *first[0].Width != 1920 {
		t.Fatal("10k snapshot not projected")
	}
	// Request a later page before intermediate pages: IDs must not reshuffle it.
	q.StartIndex = 9999
	start = time.Now()
	last, total := f.query(t, q)
	lastDuration := time.Since(start)
	again, n := f.query(t, q)
	if n != 10000 || total != 10000 || len(last) != 1 || !reflect.DeepEqual(last, again) || lastDuration >= 5*time.Second {
		t.Fatal("10k last page", total, lastDuration)
	}
	q.StartIndex = 10000
	empty, total := f.query(t, q)
	if total != 10000 || len(empty) != 0 {
		t.Fatal("count beyond end", len(empty), total)
	}
	keys := make([]string, 0, len(first))
	for i, v := range first {
		keys = append(keys, v.Id)
		if i > 0 {
			a := browseNumericID(t, first[i-1].Id)
			b := browseNumericID(t, v.Id)
			if a >= b {
				t.Fatal("numeric ID tie order")
			}
		}
	}
	before, snapshots, sources, probes := f.trace.queries.Load(), f.trace.snapshots.Load(), f.trace.sources.Load(), f.trace.probes.Load()
	start = time.Now()
	batch, n := f.query(t, nativelibrary.ItemQuery{IDs: keys, Limit: 200, SortBy: []string{"SortName"}})
	batchDuration := time.Since(start)
	batchCalls := f.trace.queries.Load() - before
	if n != 200 || len(batch) != 200 || f.trace.queries.Load()-before > 12 || f.trace.snapshots.Load()-snapshots != 1 || f.trace.sources.Load()-sources != 1 || f.trace.probes.Load()-probes != 1 || batchDuration >= 5*time.Second {
		t.Fatal("200 ID batch bound", n, f.trace.queries.Load()-before, batchDuration)
	}
	start = time.Now()
	counts, err := f.repo.Counts(f.ctx, f.user, f.permissions())
	countDuration := time.Since(start)
	if err != nil || counts.MovieCount != 10000 || countDuration >= 5*time.Second {
		t.Fatal("10k count", counts, err, countDuration)
	}
	start = time.Now()
	hints, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{SearchTerm: "SYNTHETIC MOVIE 00001", Limit: 20}})
	hintDuration := time.Since(start)
	if err != nil || hints.Hints.TotalRecordCount != 1 || len(hints.Hints.SearchHints) != 1 || hintDuration >= 5*time.Second {
		t.Fatal("10k normalized search budget", hints.Hints.TotalRecordCount, hintDuration, err)
	}
	t.Logf("T19 10000 movies SQL normalized hints: %s", hintDuration)
	t.Logf("T24-B 10000 movies WITH snapshots: cold(first browse, IDs preallocated)=%s; last-page=%s; batch(200 IDs)=%s; counts=%s; batch SQL calls=%d", cold, lastDuration, batchDuration, countDuration, batchCalls)
	// T31's endpoint budget includes T41 token validation, T42 permissions and
	// session persistence. No provider resolution is part of PlaybackInfo.
	f.exec(t, `UPDATE native_server_settings SET enabled=true,revision=revision+1`)
	secret := "synthetic-playback-perf"
	login, err := (&nativeauth.Store{DB: f.pool, Secret: secret}).Login(f.ctx, "browse.member", "Browse123", contracts.DeviceInfo{DeviceID: "perf-playback-device"})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NativeMediaHandler(config.Config{SessionKey: secret, MemberPublicURL: "https://native.synthetic.invalid"}, httpapi.Dependencies{IPBlock: fixtureIPBlock(t), DB: f.pool})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/emby/Items/"+first[0].Id+"/PlaybackInfo", nil)
	request.Header.Set("X-Emby-Token", login.AccessToken)
	request.Header.Set("User-Agent", "Infuse/8")
	response := httptest.NewRecorder()
	start = time.Now()
	handler.ServeHTTP(response, request)
	playbackDuration := time.Since(start)
	var playback dto.PlaybackInfoResponse
	if err = json.Unmarshal(response.Body.Bytes(), &playback); err != nil || response.Code != 200 || len(playback.MediaSources) != 1 || playback.PlaySessionId == "" || playbackDuration >= 6*time.Second {
		t.Fatal("10k PlaybackInfo budget", response.Code, playbackDuration, err)
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 0 {
		t.Fatal("info reserved slot")
	}
	if m := playback.MediaSources[0]; m.Container != "mkv" || m.Size == nil || m.Bitrate == nil || m.RunTimeTicks == nil || len(m.MediaStreams) != 1 || m.Width == nil || *m.Width != 1920 {
		t.Fatal("PlaybackInfo dropped probe snapshot")
	}
	t.Logf("T31 10000 movies authenticated HTTP PlaybackInfo: %s", playbackDuration)
	// T54: mixed four-way concurrency, no provider/ffx invocation. Each request
	// still performs real token/permission checks; prepared sessions consume no slot.
	type loadResult struct {
		status   int
		duration time.Duration
		kind     int
		leak     bool
	}
	results := make(chan loadResult, 8)
	var load sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		worker := worker
		load.Go(func() {
			for turn := 0; turn < 2; turn++ {
				kind := (worker + turn) % 2
				path := "/Items?Recursive=true&IncludeItemTypes=Movie&Limit=20"
				if kind == 1 {
					path = "/Items/" + first[0].Id + "/PlaybackInfo"
				}
				r := httptest.NewRequest("GET", path, nil)
				r.Header.Set("X-Emby-Token", login.AccessToken)
				r.Header.Set("User-Agent", "Infuse/8")
				w := httptest.NewRecorder()
				at := time.Now()
				handler.ServeHTTP(w, r)
				body := w.Body.String()
				results <- loadResult{w.Code, time.Since(at), kind, strings.Contains(body, "/115/") || strings.Contains(body, "pickCode") || strings.Contains(body, "strm302")}
			}
		})
	}
	load.Wait()
	close(results)
	var maximum time.Duration
	success := [2]int{}
	for v := range results {
		if v.leak || v.status != 200 || v.duration > 6*time.Second {
			t.Fatal("mixed bounded load", v)
		}
		success[v.kind]++
		maximum = max(maximum, v.duration)
	}
	t.Logf("T54 mixed HTTP concurrency=4 requests=8 browse=%d PlaybackInfo=%d max=%s", success[0], success[1], maximum)

}

func TestNativeBrowseBoundedVersionsAndRequestedHeavyFields(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 984001, "Synthetic bounded movie", "2025", `{"officialRating":"G","overview":"Stored overview"}`)
	for i := 0; i < 101; i++ {
		f.asset(t, m, "g:test-a", time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC), "mkv")
	}
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES('movie',984001,'zh-CN','{"overview":"Synthetic TMDB overview","runtimeMinutes":120,"people":[{"id":42,"name":"Synthetic Actor","type":"Actor"}]}','ready',now())`)
	q := nativelibrary.ItemQuery{ParentID: f.parents["g:test-a"], Limit: 200}
	small, n := f.query(t, q)
	if n != 1 || len(small[0].MediaSources) != 100 || small[0].Overview != "" || len(small[0].People) != 0 || !strings.Contains(small[0].MediaSources[0].Name, "最新100") {
		t.Fatal("unbounded or unsolicited heavy fields")
	}
	q.Fields = []string{"Overview", "People"}
	snapshots, sources := f.trace.snapshots.Load(), f.trace.sources.Load()
	full, n := f.query(t, q)
	if n != 1 || full[0].Overview != "Synthetic TMDB overview" || len(full[0].People) != 1 || full[0].RunTimeTicks == nil || *full[0].RunTimeTicks != 120*600000000 {
		t.Fatal("metadata projection drift", full)
	}
	if f.trace.snapshots.Load()-snapshots != 1 || f.trace.sources.Load()-sources != 1 {
		t.Fatal("payload joined per version")
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"/115/", "pickcode", "strm302", "Browse123", "synthetic-browse-key"} {
		if strings.Contains(string(raw), marker) {
			t.Fatal("private data in browse projection", marker)
		}
	}
}

func browseNumericID(t *testing.T, value string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		t.Fatal("invalid numeric ID", value, err)
	}
	return id
}
