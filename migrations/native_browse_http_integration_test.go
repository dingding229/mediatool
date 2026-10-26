package migrations_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
)

func TestNativeBrowseHTTPRealAuthenticationCompleteChain(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 987001, "Synthetic HTTP Movie", "2025", `{"officialRating":"G","genres":["Drama"]}`)
	httpMovieAsset := f.asset(t, m, "g:test-a", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "mkv")
	tv := f.library(t, "g:http-tv", "tvshows", true, false)
	hidden := f.library(t, "g:http-hidden", "movies", true, true)
	private := f.library(t, "g:http-private", "movies", false, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'series',987002,'Synthetic HTTP Show','2025','{"officialRating":"G"}')`, show)
	f.episode(t, show, "g:http-tv", 1, 1, 1)
	f.episode(t, show, "g:http-tv", 1, 2, 2)
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true,revision=revision+1`)
	var logs bytes.Buffer
	handler, err := httpapi.NativeMediaHandler(config.Config{SessionKey: "synthetic-browse-http-key", MemberPublicURL: "https://native.synthetic.invalid"}, httpapi.Dependencies{IPBlock: fixtureIPBlock(t), DB: f.pool, Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	token := ""
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("User-Agent", "Infuse/8.0")
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		request.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse", Device="Synthetic iPad", DeviceId="http-device", Version="8.0", Token="`+token+`"`)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal(readErr, closeErr)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.StatusCode, want, raw)
		}
		if method == "HEAD" {
			if len(raw) != 0 {
				t.Fatal("HEAD leaked body")
			}
			return raw
		}
		if !json.Valid(raw) {
			t.Fatal("invalid JSON")
		}
		if !strings.Contains(path, "AuthenticateByName") {
			for _, secret := range []string{token, "Browse123", "/115/", "strm302", "pickcode"} {
				if secret != "" && bytes.Contains(raw, []byte(secret)) {
					t.Fatal("private value in browse reply")
				}
			}
		}
		return raw
	}
	var login dto.AuthenticationResult
	raw := call("POST", "/emby/Users/AuthenticateByName", `{"username":"browse.member","password":"Browse123"}`, 200)
	if err := json.Unmarshal(raw, &login); err != nil || login.AccessToken == "" {
		t.Fatal("real login failed", err)
	}
	token = login.AccessToken
	decodeList := func(raw []byte) dto.QueryResult[dto.BaseItemDto] {
		t.Helper()
		var q dto.QueryResult[dto.BaseItemDto]
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&q); err != nil {
			t.Fatal(err)
		}
		return q
	}
	views := decodeList(call("GET", "/Users/"+login.User.Id+"/Views", "", 200))
	folders := decodeList(call("GET", "/emby/Library/MediaFolders", "", 200))
	if views.TotalRecordCount != 3 || folders.TotalRecordCount != 3 {
		t.Fatal("views/mediafolders mismatch", views.TotalRecordCount, folders.TotalRecordCount)
	}
	for _, v := range views.Items {
		if v.Id == hidden || v.Id == private {
			t.Fatal("ungranted view exposed")
		}
	}
	movies := decodeList(call("GET", "/emby/Users/"+login.User.Id+"/Items?parentid="+f.parents["g:test-a"]+"&lImIt=1&Fields=Overview", "", 200))
	if movies.TotalRecordCount != 1 || len(movies.Items) != 1 || movies.Items[0].Type != dto.Movie {
		t.Fatal("library movie query")
	}
	alias := decodeList(call("GET", "/Items?uSeRiD="+login.User.Id+"&ParentId="+f.parents["g:test-a"]+"&Limit=1", "", 200))
	if alias.Items[0].Id != movies.Items[0].Id {
		t.Fatal("Items alias diverged")
	}
	shows := decodeList(call("GET", "/Users/"+login.User.Id+"/Items?ParentId="+tv, "", 200))
	if shows.TotalRecordCount != 1 || shows.Items[0].Type != dto.Series {
		t.Fatal("show list")
	}
	seasons := decodeList(call("GET", "/emby/Items?UserId="+login.User.Id+"&ParentId="+shows.Items[0].Id, "", 200))
	if seasons.TotalRecordCount != 1 || seasons.Items[0].Type != dto.Season {
		t.Fatal("season list")
	}
	path := "/Items?ParentId=" + seasons.Items[0].Id + "&SortBy=IndexNumber&Limit=1"
	first := decodeList(call("GET", path, "", 200))
	second := decodeList(call("GET", "/emby"+path+"&StartIndex=1", "", 200))
	repeat := decodeList(call("GET", path+"&StartIndex=1", "", 200))
	if first.TotalRecordCount != 2 || second.TotalRecordCount != 2 || len(second.Items) != 1 || *first.Items[0].IndexNumber != 1 || *second.Items[0].IndexNumber != 2 || repeat.Items[0].Id != second.Items[0].Id {
		t.Fatal("episode stable pagination")
	}
	detail := call("GET", "/emby/Users/"+login.User.Id+"/Items/"+first.Items[0].Id, "", 200)
	var ep dto.BaseItemDto
	decoder := json.NewDecoder(bytes.NewReader(detail))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ep); err != nil || ep.Id != first.Items[0].Id || ep.ParentId != seasons.Items[0].Id {
		t.Fatal("episode detail", err)
	}
	call("GET", "/Items/"+movies.Items[0].Id, "", 200)
	call("GET", "/Items/"+tv, "", 200)
	batch := decodeList(call("GET", "/emby/Items?Ids="+movies.Items[0].Id+","+shows.Items[0].Id+","+seasons.Items[0].Id+","+ep.Id, "", 200))
	if len(batch.Items) != 4 || batch.TotalRecordCount != 4 {
		t.Fatal("HTTP mixed IDs")
	}
	var counts nativelibrary.ItemCounts
	if err := json.Unmarshal(call("GET", "/Items/Counts?UserId="+login.User.Id, "", 200), &counts); err != nil || counts.MovieCount != 1 || counts.SeriesCount != 1 || counts.EpisodeCount != 2 {
		t.Fatal(counts, err)
	}
	call("HEAD", "/emby/Items/"+ep.Id, "", 200)
	call("GET", "/Users/"+strings.Repeat("b", 32)+"/Views", "", 403)
	call("GET", "/Items?UserId="+strings.Repeat("b", 32), "", 403)
	for _, id := range []string{hidden, private, "999999999"} {
		call("GET", "/Items?ParentId="+id, "", 404)
		call("GET", "/Items/"+id, "", 404)
	}
	call("GET", "/Items?Limit=201", "", 200)
	call("GET", "/emby/Items?Years=bad", "", 400)

	f.probeSnapshot(t, httpMovieAsset, 615000000, 1920)
	probed := decodeList(call("GET", "/emby/Items?ParentId="+f.parents["g:test-a"], "", 200))
	assertSourceProbe(t, probed.Items, httpMovieAsset.asset, 1920, true)
	assertProbeHTTPGolden(t, probed)
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:http-tv'`)
	call("GET", "/Items?ParentId="+tv, "", 404)
	hiddenViews := decodeList(call("GET", "/Library/MediaFolders", "", 200))
	if hiddenViews.TotalRecordCount != 2 {
		t.Fatal("hidden view stale")
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), "Browse123") {
		t.Fatal("request logs leaked credentials")
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=false,revision=revision+1`)
	time.Sleep(settings.NativeServerCacheTTL + 10*time.Millisecond)
	call("GET", "/Items", "", 503)
	call("HEAD", "/Items/Counts", "", 503)
}
