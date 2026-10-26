package migrations_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
)

func TestNativeDiscoveryHTTPHierarchyLatestSearchFacetsSimilar(t *testing.T) {
	f := newBrowseFixture(t)
	tv := f.library(t, "g:discover-tv", "tvshows", true, false)
	f.library(t, "g:discover-tv2", "tvshows", true, false)
	private := f.library(t, "g:discover-private", "movies", false, false)
	movie := f.media(t, 988001, "星海电影", "2025", `{"officialRating":"G","originalTitle":"ＡＬＰＨＡ Ｔｅｓｔ","genres":["Drama"],"posterPath":"/synthetic-poster.jpg","backdropPath":"/synthetic-bg.jpg"}`)
	f.asset(t, movie, "g:test-a", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "mkv")
	other := f.media(t, 988002, "Synthetic Drama Two", "2024", `{"officialRating":"PG-13","genres":["Drama"]}`)
	f.asset(t, other, "g:test-b", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "mkv")
	only := f.media(t, 988003, "Synthetic Action", "2023", `{"officialRating":"G","genres":["Action"]}`)
	f.asset(t, only, "g:test-a", time.Now(), "mkv")
	for i, lib := range []string{"g:discover-private", "g:test-a"} {
		m := f.media(t, 988010+int64(i), "SecretMovie", "2099", `{"officialRating":"R","genres":["SecretGenre"]}`)
		f.asset(t, m, lib, time.Now(), "mkv")
	}
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'series',988004,'星海剧集','2025','{"officialRating":"G","genres":["Drama"],"posterPath":"/synthetic-series.jpg"}')`, show)
	f.episode(t, show, "g:discover-tv", 0, 1, 1)
	ep := f.episode(t, show, "g:discover-tv", 1, 1, 1)
	f.episode(t, show, "g:discover-tv2", 1, 1, 1)
	f.episode(t, show, "g:discover-tv2", 2, 1, 1)
	f.exec(t, `UPDATE media_assets SET created_at='2026-06-01T00:00:00Z' WHERE id=$1`, ep.asset)
	invisibleShow := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',988005,'Secret Show','{"officialRating":"R","genres":["SecretGenre"]}')`, invisibleShow)
	f.episode(t, invisibleShow, "g:discover-tv", 1, 1, 1)
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}
	id := func(kind, key string) string {
		t.Helper()
		return strconv.FormatInt(f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type=$1 AND internal_key=$2`, kind, key), 10)
	}
	seriesID, movieID := id("series", "tmdb:988004"), id("movie", "tmdb:988001")
	secretID := id("series", "tmdb:988005")
	season1, season2 := id("season", "tmdb:988004:s1"), id("season", "tmdb:988004:s2")
	ep1 := id("episode", "tmdb:988004:s1e1")
	f.exec(t, `UPDATE media_users SET max_parental_rating=65 WHERE id=$1::uuid`, f.user.ID)
	f.exec(t, `UPDATE native_server_settings SET enabled=true,revision=revision+1`)
	var logs bytes.Buffer
	handler, err := httpapi.NativeMediaHandler(config.Config{SessionKey: "synthetic-discovery-key", MemberPublicURL: "https://native.synthetic.invalid"}, httpapi.Dependencies{IPBlock: fixtureIPBlock(t), DB: f.pool, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	token := ""
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Emby-Authorization", `Emby Client="SenPlayer", DeviceId="discovery-device", Version="1.0", Token="`+token+`"`)
		r.Header.Set("User-Agent", "SenPlayer/1.0")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		if method == "HEAD" {
			if w.Body.Len() != 0 {
				t.Fatal("HEAD body")
			}
			return nil
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid JSON")
		}
		return w.Body.Bytes()
	}
	var login dto.AuthenticationResult
	if err := json.Unmarshal(call("POST", "/Users/AuthenticateByName", `{"Username":"browse.member","Pw":"Browse123"}`, 200), &login); err != nil {
		t.Fatal(err)
	}
	token = login.AccessToken
	list := func(path string) dto.QueryResult[dto.BaseItemDto] {
		t.Helper()
		var out dto.QueryResult[dto.BaseItemDto]
		if err := json.Unmarshal(call("GET", path, "", 200), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// F2: all-unknown includes must stay empty even when specialized routes
	// supply their own default types; unknown media kinds are zero-match unions.
	for _, route := range []string{"/Items?Recursive=true", "/Users/" + login.User.Id + "/Items/Resume?MediaTypes=Video", "/Shows/NextUp?SeriesId=" + seriesID, "/Shows/" + seriesID + "/Seasons?", "/Shows/" + seriesID + "/Episodes?", "/Items/" + movieID + "/Similar?"} {
		separator := "&"
		if !strings.Contains(route, "?") {
			separator = "?"
		}
		result := list(route + separator + "IncludeItemTypes=BoxSet,Trailer,Audio,Person,Playlist&ExcludeItemTypes=Unknown&SortBy=SQL%3BDROP&Limit=500")
		if len(result.Items) != 0 || result.TotalRecordCount != 0 {
			t.Fatal("unknown-only widened", route, result)
		}
	}
	for _, route := range []string{"/Items?Recursive=true&IncludeItemTypes=Movie,BoxSet", "/Shows/" + seriesID + "/Seasons?", "/Shows/" + seriesID + "/Episodes?", "/Items/" + movieID + "/Similar?"} {
		result := list(route + "&ExcludeItemTypes=Person,Playlist&SortBy=Unknown,DateCreated&SortOrder=Ascending,Descending,Descending&Limit=500")
		if len(result.Items) == 0 {
			t.Fatal("tolerant query lost known types", route)
		}
	}
	result := list("/Users/" + login.User.Id + "/Items/Resume?MediaTypes=Unknown,Audio&Limit=500")
	if len(result.Items) != 0 {
		t.Fatal("unknown MediaTypes widened")
	}
	call("GET", "/Items/Latest?Limit=500&IncludeItemTypes=BoxSet", "", 200)
	call("GET", "/Search/Hints?SearchTerm=synthetic&Limit=500", "", 200)
	seasons := list("/emby/Shows/" + seriesID + "/Seasons?uSeRiD=" + login.User.Id + "&IsMissing=true")
	if seasons.TotalRecordCount != 3 || len(seasons.Items) != 3 || *seasons.Items[0].IndexNumber != 0 {
		t.Fatal("cross-library seasons", seasons)
	}
	specials := list("/Shows/" + seriesID + "/Seasons?IsSpecialSeason=1")
	if specials.TotalRecordCount != 1 || *specials.Items[0].IndexNumber != 0 {
		t.Fatal("special seasons")
	}
	normal := list("/Shows/" + seriesID + "/Seasons?IsSpecialSeason=false")
	if normal.TotalRecordCount != 2 {
		t.Fatal("normal seasons")
	}
	all := list("/Shows/" + seriesID + "/Episodes?IsMissing=0&Limit=200")
	if all.TotalRecordCount != 3 || len(all.Items) != 3 {
		t.Fatal("episode union dedup", all)
	}
	selected := list("/emby//sHoWs/" + seriesID + "/ePiSoDeS?seasonid=" + season1 + "&Season=1&Fields=Overview")
	if selected.TotalRecordCount != 1 || len(selected.Items[0].MediaSources) != 2 {
		t.Fatal("multi-edition cross-library episode")
	}

	generic := list("/Items?SeriesId=" + seriesID + "&SeasonId=" + season1 + "&IncludeItemTypes=Episode")
	if generic.TotalRecordCount != 1 || generic.Items[0].SeasonId != season1 {
		t.Fatal("generic SeriesId/SeasonId ignored", generic)
	}
	scopedIDs := list("/Items?ParentId=" + tv + "&SeriesId=" + seriesID + "&SeasonId=" + season1 + "&IncludeItemTypes=Episode")
	if scopedIDs.TotalRecordCount != 1 || len(scopedIDs.Items[0].MediaSources) != 1 {
		t.Fatal("ID filters escaped parent library scope", scopedIDs)
	}
	call("GET", "/Shows/"+seriesID+"/Episodes?SeriesId="+secretID, "", 400)
	numeric := list("/Shows/" + seriesID + "/Episodes?Season=2")
	if numeric.TotalRecordCount != 1 || numeric.Items[0].SeasonId != season2 {
		t.Fatal("numeric season")
	}
	start := list("/Shows/" + seriesID + "/Episodes?StartItemId=" + ep1 + "&Limit=1")
	if start.TotalRecordCount != 3 || len(start.Items) != 1 || start.Items[0].Id != ep1 {
		t.Fatal("inclusive start item anchor")
	}
	for _, path := range []string{"/Shows/" + secretID + "/Seasons", "/Shows/" + secretID + "/Episodes", "/Shows/" + seriesID + "/Episodes?StartItemId=" + movieID, "/Shows/" + seriesID + "/Episodes?SeasonId=" + id("season", "tmdb:988005:s1"), "/Shows/" + seriesID + "/Episodes?Season=999", "/Shows/9999999/Seasons"} {
		call("GET", path, "", 404)
	}
	latest := func(query string) []dto.BaseItemDto {
		t.Helper()
		var out []dto.BaseItemDto
		if err := json.Unmarshal(call("GET", "/Items/Latest?"+query, "", 200), &out); err != nil {
			t.Fatal(err)
		}
		if out == nil {
			t.Fatal("null Latest")
		}
		return out
	}
	grouped := latest("IncludeItemTypes=Episode")
	if len(grouped) != 1 || grouped[0].Type != dto.Series || grouped[0].Id != seriesID {
		t.Fatal("default episode grouping", grouped)
	}
	defaultLatest := latest("")
	for _, item := range defaultLatest {
		if item.Type == dto.Episode {
			t.Fatal("default latest did not group episodes")
		}
	}
	flat := latest("IncludeItemTypes=Episode&GroupItems=false")
	if len(flat) != 3 || flat[0].Id != ep1 || flat[0].Type != dto.Episode {
		t.Fatal("latest asset time ordering", flat)
	}
	var userLatest []dto.BaseItemDto
	if err := json.Unmarshal(call("GET", "/emby/Users/"+login.User.Id+"/Items/Latest?IncludeItemTypes=Episode", "", 200), &userLatest); err != nil || len(userLatest) != 1 {
		t.Fatal("user Latest alias", err)
	}
	if scoped := latest("ParentId=" + tv + "&GroupItems=false&IncludeItemTypes=Episode"); len(scoped) != 2 {
		t.Fatal("latest parent library scope", scoped)
	}
	call("GET", "/Items/Latest?ParentId="+private, "", 404)
	for _, query := range []string{"alpha test", "ＡＬＰＨＡ ＴＥＳＴ", "星海"} {
		var hints dto.SearchHintResult
		raw := call("GET", "/emby/Search/Hints?IncludeItemTypes=Movie&SearchTerm="+url.QueryEscape(query), "", 200)
		if err := json.Unmarshal(raw, &hints); err != nil || hints.TotalRecordCount != 1 || len(hints.SearchHints) != 1 || hints.SearchHints[0].Id != movieID || hints.SearchHints[0].PrimaryImageTag == "" {
			t.Fatal("normalized search", query, hints, err)
		}
	}

	var episodeHints dto.SearchHintResult
	if err := json.Unmarshal(call("GET", "/Search/Hints?IncludeItemTypes=Episode&SearchTerm=Synthetic", "", 200), &episodeHints); err != nil || episodeHints.TotalRecordCount != 3 || episodeHints.SearchHints[0].Series != "星海剧集" || episodeHints.SearchHints[0].SeriesId != seriesID {
		t.Fatal("episode hint series fields", episodeHints, err)
	}
	var absent dto.SearchHintResult
	if err := json.Unmarshal(call("GET", "/Search/Hints?SearchTerm=Secret", "", 200), &absent); err != nil || absent.TotalRecordCount != 0 {
		t.Fatal("private/rated search leak", absent, err)
	}
	var genres dto.QueryResult[dto.NameIdPair]
	if err := json.Unmarshal(call("GET", "/Genres", "", 200), &genres); err != nil || genres.TotalRecordCount != 2 {
		t.Fatal("visible genres", genres, err)
	}
	for _, g := range genres.Items {
		if g.Id != catalog.GenreID(strings.ToLower(g.Name)) || g.Name == "SecretGenre" {
			t.Fatal("genre identity/leak", g)
		}
	}
	var filters dto.ItemFilters
	if err := json.Unmarshal(call("GET", "/Items/Filters", "", 200), &filters); err != nil || len(filters.Genres) != 2 || len(filters.Years) != 3 || len(filters.OfficialRatings) != 2 {
		t.Fatal("visible filters", filters, err)
	}
	scoped := call("GET", "/Genres?ParentId="+tv+"&IncludeItemTypes=Series", "", 200)
	if bytes.Contains(scoped, []byte("Action")) {
		t.Fatal("genres ignored parent")
	}
	similar := list("/emby/Items/" + movieID + "/Similar")
	if similar.TotalRecordCount != 1 || len(similar.Items) != 1 || similar.Items[0].Type != dto.Movie || similar.Items[0].Id == movieID {
		t.Fatal("similar type/genre/self", similar)
	}
	none := list("/Items/" + id("movie", "tmdb:988003") + "/Similar")
	if len(none.Items) != 0 || none.TotalRecordCount != 0 {
		t.Fatal("no similar data not empty", none)
	}
	call("GET", "/Items/"+id("movie", "tmdb:988011")+"/Similar", "", 404)
	for _, path := range []string{"/Search/Hints", "/Shows/" + seriesID + "/Episodes?Season=-1", "/Shows/" + seriesID + "/Seasons?IsSpecialSeason=bad"} {
		call("GET", path, "", 400)
	}
	call("HEAD", "/emby/Shows/"+seriesID+"/Episodes", "", 200)
	call("GET", "/Users/"+strings.Repeat("b", 32)+"/Items/Latest", "", 403)
	for _, secret := range []string{token, "Browse123", "/115/"} {
		if secret != "" && strings.Contains(logs.String(), secret) {
			t.Fatal("discovery logs leak")
		}
	}
	t.Log(fmt.Sprintf("T18/T19 real-token HTTP: %d cross-library seasons, %d logical episodes, English/fullwidth/Chinese SQL hints verified", seasons.TotalRecordCount, all.TotalRecordCount))
}

func TestNativeDiscoveryCandidateLimitIsExplicitAndScoped(t *testing.T) {
	f := newBrowseFixture(t)
	f.exec(t, `CREATE TEMP TABLE native_discovery_seed AS SELECT i,md5('discovery-media-'||i)::uuid media,md5('discovery-file-'||i)::uuid file,md5('discovery-asset-'||i)::uuid asset FROM generate_series(1,50001) i`)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)SELECT media,'movie',2000000+i,'Synthetic bounded search '||i,'2025','{"officialRating":"G","genres":["Drama"]}' FROM native_discovery_seed`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT file,$1,'/115/synthetic/'||i||'.mkv',1,'present' FROM native_discovery_seed`, f.storage)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,created_at)SELECT asset,file,'2026-01-01T00:00:00Z'::timestamptz FROM native_discovery_seed`)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state)SELECT asset,media,'ready' FROM native_discovery_seed`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT md5('discovery-strm-'||i)::uuid,asset,'Synthetic/'||i||'.strm','synthetic' FROM native_discovery_seed`)
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset,'g:test-a','output_directory',1,1 FROM native_discovery_seed`)
	for _, mode := range []nativelibrary.DiscoveryMode{nativelibrary.Hints, nativelibrary.Genres, nativelibrary.Filters} {
		start := time.Now()
		q := nativelibrary.ItemQuery{Limit: 20}
		if mode == nativelibrary.Hints {
			q.SearchTerm = "Synthetic bounded search"
		}
		out, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: mode, Items: q})
		if !errors.Is(err, nativelibrary.ErrDiscoveryTooBroad) || len(out.Items) != 0 || len(out.Hints.SearchHints) != 0 {
			t.Fatal("truncated/incorrect discovery total", mode, err)
		}
		t.Logf("T18/T19 explicit 50001-candidate rejection %s: %s", mode, time.Since(start))
	}
	// T53: the same oversized library must accept a selective zero-match term.
	empty, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 20, SearchTerm: "not-a-match"}})
	if err != nil || empty.Hints.TotalRecordCount != 0 {
		t.Fatal("candidate cap preceded search", err)
	}
	scoped, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{ParentID: f.parents["g:test-b"], SearchTerm: "Synthetic", Limit: 20}})
	if err != nil || scoped.Hints.TotalRecordCount != 0 {
		t.Fatal("scope was applied after candidate bound", scoped, err)
	}
	for _, q := range []nativelibrary.DiscoveryQuery{{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 20}}, {Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{SearchTerm: strings.Repeat("x", 257), Limit: 20}}} {
		if _, err := f.repo.Discover(f.ctx, f.user, f.permissions(), q); !errors.Is(err, nativelibrary.ErrInvalidQuery) {
			t.Fatal("unbounded discovery input", err)
		}
	}
}
