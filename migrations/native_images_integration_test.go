package migrations_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/httpapi"
)

// Replays SenPlayer's observed omitted-index image URLs against real catalog,
// permission and token stores. Only the fixed TMDB origin is replaced by a local
// image provider. No production accounts, media, STRM bytes or 115 requests.
func TestF4NativeClientImagesMovieSeriesSeasonEpisodePeople(t *testing.T) {
	f := newBrowseFixture(t)
	tv := f.library(t, "g:image-tv", "tvshows", true, false)
	movie := f.media(t, 940001, "Synthetic image film", "2026", `{"officialRating":"G","posterPath":"/movie.jpg"}`)
	f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',940002,'Synthetic image show','{"officialRating":"G","posterPath":"/series.jpg"}')`, show)
	f.episode(t, show, "g:image-tv", 1, 4, 4)
	for _, row := range []struct {
		kind    string
		tmdb    int
		season  any
		payload string
	}{
		{"movie", 940001, nil, `{"officialRating":"G","posterPath":"/movie.jpg","backdropPath":"/movie-bg.jpg","backdrops":["/movie-bg.jpg","/movie-bg2.jpg"],"logoPath":"/movie-logo.png","people":[{"id":1392137,"name":"Movie Actor","type":"Actor","imagePath":"/movie-person.jpg"}]}`},
		{"series", 940002, nil, `{"officialRating":"G","posterPath":"/series.jpg","backdropPath":"/series-bg.jpg","backdrops":["/series-bg.jpg","/series-bg2.jpg"],"logoPath":"/series-logo.svg","people":[{"id":2229,"name":"Series Actor","type":"Actor","imagePath":"/series-person.jpg"}]}`},
		{"season", 940002, 1, `{"posterPath":"/season.jpg","episodes":[{"number":4,"stillPath":"/still.jpg"}]}`},
	} {
		f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,state,payload,fetched_at)VALUES($1,$2,$3,'zh-CN','ready',$4,now())`, row.kind, row.tmdb, row.season, json.RawMessage(row.payload))
	}
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true`)
	requests := []string{}
	imageHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if strings.ContainsAny(r.URL.RawQuery, "?#") || r.Header.Get("X-Emby-Token") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials reached provider")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0})
	}))
	defer imageHost.Close()
	images := httpapi.NewNativeImageResolver(f.pool, nil, nil)
	images.ImageBaseURL = imageHost.URL
	accounts := &nativeauth.Store{DB: f.pool, Secret: "synthetic-image-key"}
	server, err := embyapi.NewServer(embyapi.ServerOptions{Settings: settings.NewNativeServerRepository(f.pool), PublicURL: "https://media.fixture.invalid", Images: images, Auth: accounts, Browse: f.repo, Discovery: f.repo, Permissions: func() contracts.PermissionChecker { return f.permissions() }})
	if err != nil {
		t.Fatal(err)
	}
	token := ""
	call := func(method, path string, auth bool, etag string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("User-Agent", "SenPlayer/6.2.1 (Mac OS X)")
		if auth {
			r.Header.Set("X-Emby-Authorization", `Emby Client="SenPlayer", DeviceId="image-sequence", Token="`+token+`"`)
		}
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	login, err := accounts.Login(f.ctx, "browse.member", "Browse123", contracts.DeviceInfo{DeviceID: "image-sequence"})
	if err != nil {
		t.Fatal(err)
	}
	token = login.AccessToken
	user := login.User.ID
	var views dto.QueryResult[dto.BaseItemDto]
	if err := json.Unmarshal(call("GET", "/Users/"+user+"/Views", true, "", 200).Body.Bytes(), &views); err != nil || len(views.Items) < 2 {
		t.Fatal("views", err)
	}
	lists := []dto.BaseItemDto{}
	for _, parent := range []string{f.parents["g:test-a"], tv} {
		var result dto.QueryResult[dto.BaseItemDto]
		json.Unmarshal(call("GET", "/Users/"+user+"/Items?ParentId="+parent+"&Fields=People&Limit=500", true, "", 200).Body.Bytes(), &result)
		if len(result.Items) != 1 {
			t.Fatal(result)
		}
		lists = append(lists, result.Items...)
	}
	film, series := lists[0], lists[1]
	var seasons, episodes dto.QueryResult[dto.BaseItemDto]
	json.Unmarshal(call("GET", "/Shows/"+series.Id+"/Seasons?UserId="+user, true, "", 200).Body.Bytes(), &seasons)
	if len(seasons.Items) != 1 {
		t.Fatal(seasons)
	}
	json.Unmarshal(call("GET", "/Shows/"+series.Id+"/Episodes?UserId="+user+"&SeasonId="+seasons.Items[0].Id, true, "", 200).Body.Bytes(), &episodes)
	if len(episodes.Items) != 1 {
		t.Fatal(episodes)
	}
	for _, item := range []dto.BaseItemDto{film, series, seasons.Items[0], episodes.Items[0]} {
		var detail dto.BaseItemDto
		json.Unmarshal(call("GET", "/Users/"+user+"/Items/"+item.Id+"?Fields=People", true, "", 200).Body.Bytes(), &detail)
		if detail.ImageTags["Primary"] == "" {
			t.Fatalf("missing primary: %+v", detail)
		}
		for typ, tag := range detail.ImageTags {
			path := "/Items/" + item.Id + "/Images/" + typ + "?tag=" + tag + "&maxWidth=320&quality=90"
			w := call("GET", path, true, "", 200)
			call("HEAD", path, true, "", 200)
			call("GET", path, true, w.Header().Get("ETag"), 304)
			call("GET", path, false, w.Header().Get("ETag"), 401)
		}
		for index, tag := range detail.BackdropImageTags {
			call("GET", fmt.Sprintf("/Items/%s/Images/Backdrop/%d?tag=%s&maxWidth=2000&quality=90", item.Id, index, tag), true, "", 200)
		}
		for _, p := range detail.People {
			if p.PrimaryImageTag != "" {
				call("GET", "/Items/"+p.Id+"/Images/Primary?tag="+p.PrimaryImageTag+"&maxWidth=320&quality=90", true, "", 200)
			}
		}
		if item.Type == dto.Season || item.Type == dto.Episode {
			if detail.ParentLogoImageTag == "" || detail.ParentLogoItemId != series.Id || len(detail.ParentBackdropImageTags) != 2 || detail.SeriesPrimaryImageTag == "" {
				t.Fatal("parent fallback missing", detail)
			}
			call("GET", "/Items/"+detail.ParentLogoItemId+"/Images/Logo?tag="+detail.ParentLogoImageTag, true, "", 200)
		}
	}
	for _, typ := range []string{"Backdrop", "Logo"} {
		call("GET", "/Items/"+film.Id+"/Images/"+typ, true, "", 200)
	}
	for _, path := range []string{"/Items/" + film.Id + "/Images/Backdrop/2", "/Items/" + film.Id + "/Images/Logo/1", "/Items/" + film.Id + "/Images/Banner", "/Items/" + film.Id + "/Images/Art", "/Items/1392137/Images/Primary", "/Items/" + nativelibrary.PersonID(999) + "/Images/Primary"} {
		call("GET", path, true, "", 404)
	}
	old := nativelibrary.ImageTag("/movie.jpg")
	f.exec(t, `UPDATE native_tmdb_details SET payload=jsonb_set(payload,'{posterPath}','"/movie-new.jpg"') WHERE tmdb_id=940001`)
	w := call("GET", "/Items/"+film.Id+"/Images/Primary?tag="+old, true, "\""+old+"\"", 200)
	if w.Header().Get("ETag") != "\""+nativelibrary.ImageTag("/movie-new.jpg")+"\"" || strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal(w.Header())
	}
	// Parental restrictions also scope Person images, not just title images.
	f.exec(t, `UPDATE media_users SET max_parental_rating=65 WHERE id=$1::uuid`, user)
	f.exec(t, `UPDATE native_tmdb_details SET payload=jsonb_set(payload,'{officialRating}','"R"') WHERE tmdb_id=940001`)
	call("GET", "/Items/"+nativelibrary.PersonID(1392137)+"/Images/Primary", true, "", 404)
	f.exec(t, `UPDATE media_users SET max_parental_rating=NULL WHERE id=$1::uuid`, user)
	// Live grant checks run even for an otherwise cached/304 image.
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1::uuid AND library_id='g:test-a'`, user)
	before := len(requests)
	for _, path := range []string{"/Items/" + film.Id + "/Images/Primary", "/Items/" + nativelibrary.PersonID(1392137) + "/Images/Primary"} {
		call("GET", path, true, "\""+old+"\"", 404)
	}
	if len(requests) != before {
		t.Fatal("invisible item reached image cache/provider")
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:image-tv'`)
	call("GET", "/Items/"+nativelibrary.PersonID(2229)+"/Images/Primary", true, "", 404)
}
