package migrations_test

import (
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"strings"
	"testing"
	"time"
)

func TestNativeHierarchyProjectionVisibilityCountsFallbackAndCrossLibraryIDs(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	pool := catalogTestPool(t, f)
	defer pool.Close()
	storage := jobs.UUID()
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'Hierarchy storage','clouddrive2')`, storage)
	libs := map[string]string{}
	for _, id := range []string{"g:tv-a", "g:tv-b"} {
		ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, []nativelibrary.Key{{EntityType: nativelibrary.EntityLibrary, InternalKey: id}})
		if err != nil {
			t.Fatal(err)
		}
		item := ids[nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: id}]
		libs[id] = fmt.Sprint(item)
		f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type,hidden)VALUES($1,$2,$3,$3,$3,'tvshows',false)`, id, item, id[2:])
	}
	media := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'series',970001,'Synthetic Show','2025',$2)`, media, json.RawMessage(`{"originalTitle":"Synthetic Show Original","overview":"show","genres":["Drama"]}`))
	add := func(asset, number string, episodes []int, library string, created time.Time) {
		file := jobs.UUID()
		identity := map[string]any{"mediaType": "series", "title": "Synthetic Show", "episodeDetails": []map[string]any{{"id": 970100, "seriesId": 970001, "season": 0, "number": 1, "title": "Special One", "overview": "episode", "airDate": "2025-01-02", "runtimeMinutes": 24, "stillPath": "/still.jpg"}}}
		if len(episodes) > 1 {
			identity["episodeDetails"] = []map[string]any{{"id": 970100, "seriesId": 970001, "season": 0, "number": 1, "title": "Special One", "overview": "episode", "airDate": "2025-01-02", "runtimeMinutes": 24, "stillPath": "/still.jpg"}, {"id": 970101, "seriesId": 970001, "season": 0, "number": 2, "title": "Special Two", "overview": "episode2", "airDate": "2025-01-03", "runtimeMinutes": 25, "stillPath": "/still2.jpg"}}
		}
		raw, _ := json.Marshal(identity)
		f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, file, storage, "/115/"+asset+".mkv")
		f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,media_identity,created_at)VALUES($1,$2,$3,$4)`, asset, file, raw, created)
		f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)VALUES($1,$2,0,1,$3)`, asset, media, json.RawMessage(func() string { b, _ := json.Marshal(episodes); return string(b) }()))
		f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'hierarchy')`, jobs.UUID(), asset, library+"/Special/"+asset+".strm")
		f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)VALUES($1,$2,'output_directory',1,1)`, asset, library)
	}
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	add(jobs.UUID(), "1", []int{1}, "g:tv-a", now)
	add(jobs.UUID(), "1", []int{1}, "g:tv-a", now.Add(time.Minute))
	add(jobs.UUID(), "1", []int{1, 2}, "g:tv-a", now.Add(2*time.Minute))
	add(jobs.UUID(), "1", []int{1}, "g:tv-b", now)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES('series',970001,'zh-CN',$1,'ready',now())`, json.RawMessage(`{"name":"Synthetic Show","posterPath":"/show.jpg","backdropPath":"/backdrop.jpg","premiereDate":"2025-01-01","status":"Ended"}`))
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at)VALUES('season',970001,0,'zh-CN',$1,'ready',now())`, json.RawMessage(`{"name":"特别篇","posterPath":"/special.jpg","episodes":[{"number":1,"runtimeMinutes":24},{"number":2,"runtimeMinutes":25}]}`))
	repo := catalog.HierarchyRepository{DB: pool}
	if _, _, err := repo.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-b"}}, Limit: 200}); err == nil {
		t.Fatal("invisible library was accepted")
	}
	if _, _, err := repo.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-a"}}, Limit: 201}); err == nil {
		t.Fatal("series page bound missing")
	}
	series, total, err := repo.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-a"}}, Limit: 200})
	if err != nil || total != 1 || len(series) != 1 || series[0].ChildCount == nil || *series[0].ChildCount != 1 || series[0].RecursiveItemCount == nil || *series[0].RecursiveItemCount != 1 {
		t.Fatal(series, total, err)
	}
	seasons, err := repo.ListSeasons(f.ctx, nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-a"}}, series[0].Id)
	if err != nil || len(seasons) != 1 || seasons[0].IndexNumber == nil || *seasons[0].IndexNumber != 0 || seasons[0].Name != "特别篇" {
		t.Fatal(seasons, err)
	}
	// Same season/episode numbers in another show must never contaminate this
	// show's logical page or its MediaSources (T17 entrypoint regression).
	originalMedia := media
	media = jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',970002,'Synthetic Other Show','{}')`, media)
	add(jobs.UUID(), "1", []int{1}, "g:tv-a", now.Add(3*time.Minute))
	media = originalMedia
	eps, total, err := repo.ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-a"}}, SeriesID: series[0].Id, SeasonID: seasons[0].Id, Limit: 200})
	if err != nil || total != 1 || len(eps) != 1 || eps[0].IndexNumberEnd == nil || *eps[0].IndexNumberEnd != 2 || len(eps[0].MediaSources) != 3 {
		t.Fatal(eps, total, err)
	}
	otherSeries, _, err := repo.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-b", AllowedIDs: []string{"g:tv-b"}}, Limit: 200})
	if err != nil || len(otherSeries) != 1 || otherSeries[0].Id != series[0].Id {
		t.Fatal("cross-library ID", otherSeries, series)
	}
	_, err = repo.GetEpisode(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:tv-a", AllowedIDs: []string{"g:tv-a"}}, SeriesID: series[0].Id, SeasonID: seasons[0].Id, Limit: 200}, eps[0].Id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeLongSeriesEpisodeKeyPaginationAndSinglePayloadRead(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	pool := catalogTestPool(t, f)
	defer pool.Close()
	storage, media := jobs.UUID(), jobs.UUID()
	f.exec(t, `INSERT INTO storage_connections(id,name,provider) VALUES($1,'Long series storage','clouddrive2')`, storage)
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, []nativelibrary.Key{{EntityType: nativelibrary.EntityLibrary, InternalKey: "g:long-tv"}})
	if err != nil {
		t.Fatal(err)
	}
	libraryItem := ids[nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: "g:long-tv"}]
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type) VALUES('g:long-tv',$1,'long-tv','Long TV','电视剧','tvshows')`, libraryItem)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata) VALUES($1,'series',980001,'Long Synthetic Show','2025','{}')`, media)
	large := json.RawMessage(`{"name":"Long Synthetic Show","overview":"` + strings.Repeat("x", 3<<20) + `"}`)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at) VALUES('series',980001,'zh-CN',$1,'ready',now())`, large)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at) VALUES('season',980001,1,'zh-CN',$1,'ready',now())`, json.RawMessage(`{"name":"Season One"}`))
	query := `WITH files AS (SELECT gen_random_uuid() id, g FROM generate_series(1,3000) g),
 cf AS (INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state) SELECT id,$1,'/115/long-'||g||'.mkv',1,'present' FROM files RETURNING id),
 ma AS (INSERT INTO media_assets(id,cloud_file_id,media_identity,created_at) SELECT id,id,'{}',now() FROM cf RETURNING id),
 mb AS (INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes) SELECT id,$2,1,row_number() OVER(),jsonb_build_array(row_number() OVER()) FROM ma RETURNING asset_id),
 se AS (INSERT INTO strm_entries(id,asset_id,output_path,key_reference) SELECT gen_random_uuid(),asset_id,'电视剧/长篇/'||asset_id||'.strm','long' FROM mb RETURNING asset_id),
 al AS (INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision) SELECT asset_id,'g:long-tv','output_directory',1,1 FROM se RETURNING asset_id)
 SELECT count(*) FROM al`
	var inserted int
	if err := f.db.QueryRowContext(f.ctx, query, storage, media).Scan(&inserted); err != nil || inserted != 3000 {
		t.Fatal(inserted, err)
	}
	repo := catalog.HierarchyRepository{DB: pool}
	started := time.Now()
	series, total, err := repo.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:long-tv", AllowedIDs: []string{"g:long-tv"}}, Limit: 200})
	if err != nil || total != 1 || len(series) != 1 {
		t.Fatal(series, total, err)
	}
	list, total, err := repo.ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:long-tv", AllowedIDs: []string{"g:long-tv"}}, SeriesID: series[0].Id, Limit: 200})
	if err != nil || total != 3000 || len(list) != 200 || list[0].IndexNumber == nil || *list[0].IndexNumber != 1 {
		t.Fatal(total, len(list), err)
	}
	last, lastTotal, err := repo.ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:long-tv", AllowedIDs: []string{"g:long-tv"}}, SeriesID: series[0].Id, StartIndex: 2800, Limit: 200})
	if err != nil || lastTotal != 3000 || len(last) != 200 || last[199].IndexNumber == nil || *last[199].IndexNumber != 3000 {
		t.Fatal(lastTotal, len(last), err)
	}
	elapsed := time.Since(started)
	t.Logf("T25-D 3000-episode series + first/last 200-item pages: %s", elapsed)
	if elapsed > 6*time.Second {
		t.Fatalf("long series query exceeded budget: %s", elapsed)
	}
	direct, err := repo.GetEpisode(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:long-tv", AllowedIDs: []string{"g:long-tv"}}, SeriesID: series[0].Id, Limit: 1}, last[199].Id)
	if err != nil || direct.Id != last[199].Id {
		t.Fatal("direct internal-key lookup failed", direct, err)
	}
}
