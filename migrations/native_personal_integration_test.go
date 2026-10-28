package migrations_test

import (
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"strings"
	"testing"
	"time"
)

func personalRead(t *testing.T, f *playbackFixture, id string) dto.UserItemDataDto {
	t.Helper()
	w := f.call("GET", "/Users/"+f.user.ID+"/Items/"+id+"/UserData", f.token, "Infuse/8")
	var d dto.UserItemDataDto
	if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil || w.Code != 200 {
		t.Fatal(w.Code, e, w.Body.String())
	}
	return d
}
func TestNativePersonalLeafWritesFiltersResume(t *testing.T) {
	f := newReportFixture(t)
	f.probeSnapshot(t, f.a, 6000000000, 1920)
	d := personalRead(t, f, f.item)
	if d.Key != f.item || d.PlaybackPositionTicks == nil || *d.PlaybackPositionTicks != 0 || d.PlayCount == nil || *d.PlayCount != 0 || d.PlayedPercentage == nil {
		t.Fatal(d)
	}
	path := "/Users/" + f.user.ID + "/Items/" + f.item + "/UserData"
	w := reportHTTP(f, "POST", path, `{"PlaybackPositionTicks":1200000000,"IsFavorite":true,"PlayCount":3,"LastPlayedDate":"2026-10-17T00:00:00Z"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, f.item)
	if !d.IsFavorite || *d.PlayCount != 3 || d.LastPlayedDate == nil || *d.PlaybackPositionTicks != 1200000000 {
		t.Fatal(d)
	}
	items, total, e := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, IsFavorite: boolPtr(true), IsResumable: boolPtr(true), Limit: 200})
	if e != nil || total != 1 || len(items) != 1 || items[0].Id != f.item {
		t.Fatal(items, total, e)
	}
	w = f.call("GET", "/Users/"+f.user.ID+"/Items/Resume?MediaTypes=Video&Limit=1", f.token, "Infuse/8")
	var page dto.QueryResult[dto.BaseItemDto]
	if e = json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 1 || page.Items[0].Id != f.item {
		t.Fatal(w.Code, e, w.Body.String())
	}
	for i := 0; i < 2; i++ {
		w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+f.item+"?DatePlayed=2026-10-16T00:00:00Z", "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	d = personalRead(t, f, f.item)
	if !d.Played || *d.PlayCount != 4 || *d.PlaybackPositionTicks != 0 || d.LastPlayedDate.Time().UTC().Format("2006-01-02") != "2026-10-16" {
		t.Fatal(d)
	}
	w = reportHTTP(f, "DELETE", "/Users/"+f.user.ID+"/PlayedItems/"+f.item, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, f.item)
	if d.Played || *d.PlayCount != 0 || d.LastPlayedDate != nil || !d.IsFavorite {
		t.Fatal(d)
	}
}
func TestNativePersonalHierarchyDerivedAndNextUp(t *testing.T) {
	f := newReportFixture(t)
	f.library(t, "g:personal-tv", "tvshows", true, false)
	m := f.media(t, 3536001, "Synthetic Personal Show", "2026", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	for _, pair := range [][2]int{{0, 1}, {1, 1}, {1, 2}, {2, 1}} {
		f.episode(t, m, "g:personal-tv", pair[0], pair[1], pair[1])
	}
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	show := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:3536001'`)
	season := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='season' AND internal_key='tmdb:3536001:s1'`)
	first := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536001:s1e1'`)
	next := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536001:s1e2'`)
	d := personalRead(t, f, show)
	if d.UnplayedItemCount == nil || *d.UnplayedItemCount != 4 || d.Played {
		t.Fatal(d)
	}
	w := reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+first, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = f.call("GET", "/Shows/NextUp?SeriesId="+show, f.token, "Infuse/8")
	var page dto.QueryResult[dto.BaseItemDto]
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 1 || page.Items[0].Id != next {
		t.Fatal(w.Code, e, w.Body.String())
	}
	w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+season, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, season)
	if !d.Played || *d.UnplayedItemCount != 0 {
		t.Fatal(d)
	}
	d = personalRead(t, f, show)
	if d.Played || *d.UnplayedItemCount != 2 {
		t.Fatal(d)
	}
	w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+show, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, show)
	if !d.Played || *d.UnplayedItemCount != 0 {
		t.Fatal(d)
	}
	w = reportHTTP(f, "DELETE", "/Users/"+f.user.ID+"/PlayedItems/"+show, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, show)
	if d.Played || *d.UnplayedItemCount != 4 {
		t.Fatal(d)
	}
	// Dates genuinely older than the documented 90-day recent window are excluded.
	f.exec(t, `UPDATE media_user_item_data SET last_played_at=$1,played=(item_id=$2::bigint)`, time.Now().Add(-100*24*time.Hour), first)
	w = f.call("GET", "/Shows/NextUp", f.token, "Infuse/8")
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 0 {
		t.Fatal(w.Code, e, w.Body.String())
	}
}

func TestNativePersonalScopeSortingPaginationAndSQLBatch(t *testing.T) {
	f := newReportFixture(t)
	second := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:3132002'`)
	for i, id := range []string{f.item, second} {
		w := reportHTTP(f, "POST", "/Users/"+f.user.ID+"/Items/"+id+"/UserData", fmt.Sprintf(`{"PlaybackPositionTicks":1000000000,"PlayCount":%d,"LastPlayedDate":"2026-10-%02dT00:00:00Z"}`, i+1, 10+i))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	q := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, SortBy: []string{"DatePlayed"}, SortOrder: []string{"Descending"}, Limit: 1, IsPlayed: boolPtr(false)}
	items, total, e := f.repo.Query(f.ctx, f.user, f.permissions(), q)
	if e != nil || total != 2 || len(items) != 1 || items[0].Id != second {
		t.Fatal(items, total, e)
	}
	q.StartIndex = 1
	q.SortBy = []string{"PlayCount"}
	items, total, e = f.repo.Query(f.ctx, f.user, f.permissions(), q)
	if e != nil || total != 2 || len(items) != 1 || items[0].Id != f.item {
		t.Fatal(items, total, e)
	}
	// No row-by-row personalization SQL: two items cost exactly the same queries as one.
	q.StartIndex = 0
	before := f.trace.queries.Load()
	_, _, e = f.repo.Query(f.ctx, f.user, f.permissions(), q)
	single := f.trace.queries.Load() - before
	q.Limit = 200
	before = f.trace.queries.Load()
	items, _, e = f.repo.Query(f.ctx, f.user, f.permissions(), q)
	batch := f.trace.queries.Load() - before
	if e != nil || len(items) != 2 || single != batch || batch > 12 {
		t.Fatal("userdata SQL batch", single, batch, e)
	}
	for _, v := range items {
		if v.UserData == nil || v.UserData.Key != v.Id || v.UserData.PlayCount == nil {
			t.Fatal(v.UserData)
		}
	}
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1::uuid AND library_id='g:test-b'`, f.user.ID)
	for _, method := range []string{"GET", "POST"} {
		w := reportHTTP(f, method, "/Users/"+f.user.ID+"/Items/"+second+"/UserData", `{"IsFavorite":true}`)
		if w.Code != 404 {
			t.Fatal("invisible data access", w.Code)
		}
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	if w := reportHTTP(f, "POST", "/Users/"+f.user.ID+"/FavoriteItems/"+f.item, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := f.call("GET", "/Users/"+f.user.ID+"/Items/Resume?MediaTypes=Audio", f.token, "Infuse/8"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	t.Logf("T35 UserData page SQL: one=%d batch=%d", single, batch)
}
func TestNativePersonalVisibleEpisodeAggregationAndSpecials(t *testing.T) {
	f := newReportFixture(t)
	f.library(t, "g:p-visible", "tvshows", true, false)
	f.library(t, "g:p-secret", "tvshows", false, false)
	m := f.media(t, 3536002, "Synthetic Scoped Show", "2026", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	for _, x := range []struct {
		lib  string
		s, e int
	}{{"g:p-visible", 0, 1}, {"g:p-visible", 1, 1}, {"g:p-secret", 1, 2}} {
		f.episode(t, m, x.lib, x.s, x.e, x.e)
	}
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	show := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:3536002'`)
	first := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536002:s1e1'`)
	special := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536002:s0e1'`)
	d := personalRead(t, f, show)
	if *d.UnplayedItemCount != 2 {
		t.Fatal(d)
	}
	w := reportHTTP(f, "POST", "/Users/"+f.user.ID+"/Items/"+first+"/UserData", `{"PlaybackPositionTicks":1000000000}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page dto.QueryResult[dto.BaseItemDto]
	for _, x := range []struct{ query, want string }{{"", first}, {"&EnableSpecials=true", special}} {
		w = f.call("GET", "/Shows/NextUp?SeriesId="+show+x.query, f.token, "Infuse/8")
		if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 1 || page.Items[0].Id != x.want {
			t.Fatal(w.Code, w.Body.String(), e)
		}
	}
	w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+show, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	d = personalRead(t, f, show)
	if !d.Played || *d.UnplayedItemCount != 0 || f.integer(t, `SELECT count(*) FROM media_user_item_data d JOIN native_item_ids n ON n.id=d.item_id WHERE n.internal_key='tmdb:3536002:s1e2'`) != 0 {
		t.Fatal("bulk crossed visibility boundary", d)
	}
	// Rating metadata does not block user-data writes on an authorized title.
	f.exec(t, `UPDATE media_items SET metadata='{"officialRating":"R"}' WHERE id=$1`, m)
	if w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+show, ""); w.Code != 200 {
		t.Fatal("rated title write rejected", w.Code)
	}
}

func TestNativePersonalBulk5000LimitIsAtomic(t *testing.T) {
	f := newReportFixture(t)
	f.library(t, "g:p-bulk", "tvshows", true, false)
	m := f.media(t, 3536003, "Synthetic Bulk Show", "2026", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	f.exec(t, `WITH files AS(SELECT gen_random_uuid() id,g FROM generate_series(1,5001)g),
 cf AS(INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT id,$1,'/115/synthetic/bulk/'||g||'.mkv',1,'present' FROM files RETURNING id),
 ma AS(INSERT INTO media_assets(id,cloud_file_id,media_identity)SELECT cf.id,cf.id,'{}' FROM cf RETURNING id),
 mb AS(INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)SELECT ma.id,$2,1,g,jsonb_build_array(g) FROM ma JOIN files ON files.id=ma.id RETURNING asset_id),
 se AS(INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT gen_random_uuid(),asset_id,'Synthetic/Bulk/'||asset_id||'.strm','synthetic' FROM mb RETURNING asset_id)
 INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset_id,'g:p-bulk','output_directory',1,1 FROM se`, f.storage, m)
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES('series',3536003,'zh-CN',jsonb_build_object('officialRating','G','overview',repeat('x',1048576)),'ready',now())`)
	show := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:3536003'`)
	w := reportHTTP(f, "POST", "/Users/"+f.user.ID+"/Items/"+show+"/UserData", `{"Played":true,"IsFavorite":true}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "5000") {
		t.Fatal("bulk overflow", w.Code, w.Body.String())
	}
	if f.integer(t, `SELECT count(*) FROM media_user_item_data`) != 0 {
		t.Fatal("partial write on overflow")
	}
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id IN(SELECT cloud_file_id FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id WHERE b.media_id=$1 AND b.episode=5001)`, m)
	f.episode(t, m, "g:p-bulk", 1, 1, 1) // extra version must not count as an extra logical episode
	f.trace.phaseMu.Lock()
	f.trace.timings = map[string]time.Duration{}
	f.trace.phaseMu.Unlock()
	w = reportHTTP(f, "POST", "/Users/"+f.user.ID+"/PlayedItems/"+show, "")
	t.Logf("T35 bulk timings: %v", f.trace.timings)
	if w.Code != 200 {
		t.Fatal("5000 exact boundary", w.Code, w.Body.String())
	}
	if f.integer(t, `SELECT count(*) FROM media_user_item_data WHERE played`) != 5000 {
		t.Fatal("logical bulk count")
	}
	d := personalRead(t, f, show)
	if !d.Played || *d.UnplayedItemCount != 0 {
		t.Fatal(d)
	}
	// NextUp ranks lightweight keys first, not 5000 repeated episode payloads.
	f.exec(t, `UPDATE media_user_item_data d SET played=false WHERE d.item_id IN(SELECT id FROM native_item_ids WHERE entity_type='episode' AND internal_key LIKE 'tmdb:3536003:s1e%' AND internal_key<>'tmdb:3536003:s1e1')`)
	start := time.Now()
	w = f.call("GET", "/Shows/NextUp?SeriesId="+show, f.token, "Infuse/8")
	var page dto.QueryResult[dto.BaseItemDto]
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 1 || page.Items[0].IndexNumber == nil || *page.Items[0].IndexNumber != 2 {
		t.Fatal("long nextup", w.Code, e, w.Body.String())
	}
	t.Logf("T35 5000-episode NextUp: %s", time.Since(start))
}

func TestNativePersonalNextUpRecentSeriesBound(t *testing.T) {
	f := newReportFixture(t)
	f.library(t, "g:p-next-cap", "tvshows", true, false)
	f.exec(t, `CREATE TEMP TABLE personal_recent_seed AS SELECT g,md5('personal-series-'||g)::uuid media FROM generate_series(1,501)g`)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata) SELECT media,'series',3600000+g,'Synthetic Recent '||g,'{"officialRating":"G"}' FROM personal_recent_seed`)
	f.exec(t, `WITH files AS(SELECT md5('personal-next-file-'||g||'-'||e)::uuid id,media,e FROM personal_recent_seed CROSS JOIN generate_series(1,2)e),
 cf AS(INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT id,$1,'/115/synthetic/'||id||'.mkv',1,'present' FROM files RETURNING id),
 ma AS(INSERT INTO media_assets(id,cloud_file_id)SELECT id,id FROM cf RETURNING id),
 mb AS(INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)SELECT ma.id,files.media,1,files.e,jsonb_build_array(files.e) FROM ma JOIN files ON files.id=ma.id RETURNING asset_id),
 se AS(INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT gen_random_uuid(),asset_id,'Synthetic/Next/'||asset_id||'.strm','synthetic' FROM mb RETURNING asset_id)
 INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset_id,'g:p-next-cap','output_directory',1,1 FROM se`, f.storage)
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,played,play_count,last_played_at)SELECT $1::uuid,id,true,1,now() FROM native_item_ids WHERE entity_type='episode' AND internal_key LIKE 'tmdb:360%:s1e1'`, f.user.ID)
	if w := f.call("GET", "/Shows/NextUp?Limit=1", f.token, "Infuse/8"); w.Code != 400 {
		t.Fatal("recent series cap", w.Code, w.Body.String())
	}
	show := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:3600001'`)
	var page dto.QueryResult[dto.BaseItemDto]
	w := f.call("GET", "/Shows/NextUp?SeriesId="+show, f.token, "Infuse/8")
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.TotalRecordCount != 1 || page.Items[0].IndexNumber == nil || *page.Items[0].IndexNumber != 2 {
		t.Fatal("scoped recent series", w.Code, e, w.Body.String())
	}
	f.exec(t, `UPDATE media_user_item_data d SET last_played_at=now()-interval '100 days' WHERE d.item_id IN(SELECT id FROM native_item_ids WHERE entity_type='episode' AND internal_key NOT IN('tmdb:3600001:s1e1','tmdb:3600002:s1e1'))`)
	counts := []int64{}
	for _, limit := range []int{1, 200} {
		before := f.trace.queries.Load()
		out, e := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.NextUp, Items: nativelibrary.ItemQuery{Limit: limit}})
		if e != nil || out.Total != 2 || len(out.Items) != min(limit, 2) {
			t.Fatal(out.Total, len(out.Items), e)
		}
		counts = append(counts, f.trace.queries.Load()-before)
	}
	if counts[0] != counts[1] || counts[1] > 12 {
		t.Fatal("nextup per-item SQL", counts)
	}
	t.Logf("T35 NextUp SQL: one=%d batch=%d", counts[0], counts[1])
}
