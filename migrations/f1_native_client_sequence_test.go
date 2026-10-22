package migrations_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
)

// Replays the actual wire parameter combinations, through authentication and
// SQL permissions, not a mock that always returns the same populated list.
func TestF1NativeSenPlayerBrowseSequence(t *testing.T) {
	f := newBrowseFixture(t)
	movieRoot := f.media(t, 981001, "F1 synthetic film", "2026", `{}`)
	f.asset(t, movieRoot, "g:test-a", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "mkv")
	tvLibrary := f.library(t, "g:f1-tv", "tvshows", true, false)
	privateLibrary := f.library(t, "g:f1-private", "movies", false, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'series',981002,'F1 synthetic show','2026','{}')`, show)
	f.episode(t, show, "g:f1-tv", 1, 1, 1)
	f.episode(t, show, "g:f1-tv", 1, 2, 2)
	private := f.media(t, 981003, "F1 inaccessible film", "2026", `{}`)
	f.asset(t, private, "g:f1-private", time.Now(), "mkv")
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
	handler, err := httpapi.NativeMediaHandler(config.Config{SessionKey: "f1-synthetic-key", MemberPublicURL: "https://native.synthetic.invalid"}, httpapi.Dependencies{DB: f.pool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	var fixture struct {
		UserAgent string
		Steps     []struct {
			Name, Path, Type, Shape, Source string
			AllowEmpty                      bool
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "internal", "modules", "embyapi", "testdata", "client_sequences", "senplayer-6.2.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	token := ""
	call := func(t *testing.T, method, path, body, agent string, want int) []byte {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("User-Agent", agent)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Emby-Authorization", `MediaBrowser Client="SenPlayer", Device="F1 test", DeviceId="f1-synthetic", Version="6.2.1", Token="`+token+`"`)
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s: status %d want %d: %s", method, path, res.StatusCode, want, b)
		}
		return b
	}
	var login struct {
		AccessToken string
		User        dto.UserDto
	}
	if err := json.Unmarshal(call(t, "POST", "/emby/Users/AuthenticateByName", `{"Username":"browse.member","Pw":"Browse123"}`, fixture.UserAgent, 200), &login); err != nil {
		t.Fatal(err)
	}
	token = login.AccessToken
	if token == "" || login.User.Id == "" {
		t.Fatal("missing login identity")
	}
	vars := map[string]string{"{user}": login.User.Id, "{movieLibrary}": f.parents["g:test-a"], "{tvLibrary}": tvLibrary,
		"{movie}": f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:981001'`), "{series}": f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:981002'`), "{season}": f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='season' AND internal_key='tmdb:981002:s1'`)}
	expand := func(path string) string {
		for k, v := range vars {
			path = strings.ReplaceAll(path, k, v)
		}
		return path
	}
	privateMovie := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:981003'`)
	// A second unrelated UA proves this isn't a client-name special case.
	for _, agent := range []string{fixture.UserAgent, "F1-Generic-Emby-Client/1.0"} {
		for _, step := range fixture.Steps {
			t.Run(agent+"/"+step.Name, func(t *testing.T) {
				b := call(t, "GET", expand(step.Path), "", agent, 200)
				var items []dto.BaseItemDto
				switch step.Shape {
				case "query":
					var result dto.QueryResult[dto.BaseItemDto]
					if err := json.Unmarshal(b, &result); err != nil {
						t.Fatal(err)
					}
					items = result.Items
					if result.TotalRecordCount != len(items) {
						t.Fatalf("count=%d rows=%d", result.TotalRecordCount, len(items))
					}
				case "array":
					if err := json.Unmarshal(b, &items); err != nil {
						t.Fatal(err)
					}
				case "detail":
					var item dto.BaseItemDto
					if err := json.Unmarshal(b, &item); err != nil {
						t.Fatal(err)
					}
					items = []dto.BaseItemDto{item}
				default:
					t.Fatal("unknown response shape")
				}
				if items == nil || len(items) == 0 && !step.AllowEmpty {
					t.Fatalf("empty sequence response: %s", b)
				}
				for _, item := range items {
					if string(item.Type) != step.Type || item.Id == "" || item.Id == privateLibrary || item.Id == privateMovie {
						t.Fatalf("wrong or unauthorized item: %+v", item)
					}
					if step.Type == "CollectionFolder" && item.CollectionType != "movies" && item.CollectionType != "tvshows" {
						t.Fatal("missing collection type")
					}
					if step.Type == "Series" && (item.ChildCount == nil || *item.ChildCount != 1) {
						t.Fatal("wrong series ChildCount")
					}
					if step.Type == "Season" && (item.ChildCount == nil || *item.ChildCount != 2) {
						t.Fatal("wrong season ChildCount")
					}
					if step.Type == "Season" && item.SeriesId != vars["{series}"] {
						t.Fatal("season detached from series")
					}
					if step.Type == "Episode" && (item.SeriesId != vars["{series}"] || item.SeasonId != vars["{season}"]) {
						t.Fatal("episode detached from hierarchy")
					}
				}
			})
		}
	}
	for _, q := range []string{"IncludeItemTypes=Video", "IncludeItemTypes=MusicVideo,MusicAlbum", "IncludeItemTypes=library", "IncludeItemTypes=BoxSet,Trailer,Person,Playlist"} {
		var r dto.QueryResult[dto.BaseItemDto]
		if err := json.Unmarshal(call(t, "GET", expand("/Items?UserId={user}&ParentId={movieLibrary}&Recursive=true&"+q), "", fixture.UserAgent, 200), &r); err != nil || r.TotalRecordCount != 0 || len(r.Items) != 0 {
			t.Fatal("unsupported-only filter broadened", r, err)
		}
	}
	for _, q := range []string{"IncludeItemTypes=Movie,Video&ExcludeItemTypes=Video", "IncludeItemTypes=Movie,BoxSet&ExcludeItemTypes=library,Unknown", "includeitemtypes=movie,video&sortby=datelastcontentadded,datecreated,sortname&sortorder=descending&enableUserData=false"} {
		var r dto.QueryResult[dto.BaseItemDto]
		if err := json.Unmarshal(call(t, "GET", expand("/Items?UserId={user}&ParentId={movieLibrary}&Recursive=true&"+q), "", fixture.UserAgent, 200), &r); err != nil || r.TotalRecordCount != 1 || len(r.Items) != 1 {
			t.Fatal("standard union lost movie", r, err)
		}
	}
	var page dto.QueryResult[dto.BaseItemDto]
	if err := json.Unmarshal(call(t, "GET", expand("/Items?UserId={user}&ParentId={movieLibrary}&IncludeItemTypes=Movie,Video&Recursive=true&StartIndex=1&Limit=30"), "", fixture.UserAgent, 200), &page); err != nil || page.TotalRecordCount != 1 || len(page.Items) != 0 || page.StartIndex == nil || *page.StartIndex != 1 {
		t.Fatal("pagination lost total/start", page, err)
	}
	call(t, "GET", expand("/Items?UserId={user}&IncludeItemTypes=Movie,Unknown&Recursive=true"), "", fixture.UserAgent, 200)
	call(t, "GET", expand("/Items?UserId={user}&Limit=201"), "", fixture.UserAgent, 200)
	// Permission revocation and hiding still fail closed with mixed type filters.
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1 AND library_id='g:f1-tv'`, f.user.ID)
	call(t, "GET", expand("/Items?UserId={user}&ParentId={tvLibrary}&Recursive=true&IncludeItemTypes=Series,Movie,Video"), "", fixture.UserAgent, 404)
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	call(t, "GET", expand("/Items?UserId={user}&ParentId={movieLibrary}&Recursive=true&IncludeItemTypes=Movie,Video"), "", fixture.UserAgent, 404)
}

func TestF1LastContentAddedUsesLatestVisibleMember(t *testing.T) {
	f := newBrowseFixture(t)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := f.media(t, 982001, "Older movie with new version", "2026", `{}`)
	f.asset(t, older, "g:test-a", stamp, "mkv")
	f.asset(t, older, "g:test-a", stamp.Add(3*time.Hour), "mp4")
	newer := f.media(t, 982002, "Newer movie", "2026", `{}`)
	f.asset(t, newer, "g:test-a", stamp.Add(time.Hour), "mkv")
	// A newer source in another granted library may not affect this ParentId.
	f.asset(t, newer, "g:test-b", stamp.Add(4*time.Hour), "mp4")
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}
	query := func(sort string) []dto.BaseItemDto {
		t.Helper()
		v, err := embyapi.ParseValues("ParentId=" + f.parents["g:test-a"] + "&Recursive=true&IncludeItemTypes=Movie,Video&SortBy=" + sort + ",SortName&SortOrder=Descending&Limit=30")
		if err != nil {
			t.Fatal(err)
		}
		q, err := embyapi.ParseQuery(v)
		if err != nil {
			t.Fatal(err)
		}
		items, total := f.query(t, q.ItemQuery())
		if total != 2 || len(items) != 2 {
			t.Fatal(total, items)
		}
		return items
	}
	if items := query("DateLastContentAdded"); items[0].Name != "Older movie with new version" {
		t.Fatal("not latest visible content order", items)
	}
	if items := query("DateCreated"); items[0].Name != "Newer movie" {
		t.Fatal("creation sort semantics changed", items)
	}
}
