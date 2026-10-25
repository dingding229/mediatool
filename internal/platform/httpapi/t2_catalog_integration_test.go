package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestT2ManagedDetailGoldenAndDeletedPlaybackRoute(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	exec := func(q string) {
		t.Helper()
		if _, e := s.DB.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO storage_connections(id,name,provider)VALUES('aaaaaaaa-0000-4000-8000-000000000001','fixture','clouddrive2');
 INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES('aaaaaaaa-0000-4000-8000-000000000002','aaaaaaaa-0000-4000-8000-000000000001','/PRIVATE-CLOUD/movie.mkv',7960117968);
 INSERT INTO media_assets(id,cloud_file_id)VALUES('aaaaaaaa-0000-4000-8000-000000000003','aaaaaaaa-0000-4000-8000-000000000002');
 INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES('aaaaaaaa-0000-4000-8000-000000000004','movie',100,'Synthetic Movie','2026','{"countries":["US"],"originalLanguage":"en"}');
 INSERT INTO media_asset_bindings(asset_id,media_id)VALUES('aaaaaaaa-0000-4000-8000-000000000003','aaaaaaaa-0000-4000-8000-000000000004');
 INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,payload,fetched_at)VALUES('movie',100,'{"communityRating":6.681,"overview":"Synthetic synopsis"}',now());
 INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,result,probed_at)VALUES('aaaaaaaa-0000-4000-8000-000000000002','fixture','fixture','{"schema_version":1,"streams":[{"type":"Video","width":1920,"height":872}]}',now())`)
	c := cfg()
	h := New(c, Dependencies{DB: s.DB, AuthSessions: testSessionReader{}})
	w := invoke(h, "GET", "/api/v1/media/library/managed/aaaaaaaa-0000-4000-8000-000000000003", "", c.SessionKey)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	var got, want any
	if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("testdata/contracts/t2-managed-detail.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &want); e != nil {
		t.Fatal(e)
	}
	actual, _ := json.Marshal(got)
	expected, _ := json.Marshal(want)
	if string(actual) != string(expected) {
		t.Fatalf("managed detail contract differs:\n%s", actual)
	}
	if strings.Contains(string(actual), "PRIVATE-CLOUD") {
		t.Fatal("storage details escaped projection")
	}
	for _, method := range []string{"GET", "HEAD"} {
		removed := invoke(h, method, "/api/v1/media/assets/aaaaaaaa-0000-4000-8000-000000000003/strm", "", c.SessionKey)
		if removed.Code != 404 {
			t.Fatal("removed management playback route remains", method, removed.Code)
		}
	}
	unauth := httptest.NewRecorder()
	h.ServeHTTP(unauth, httptest.NewRequest("GET", "/api/v1/media/library/managed/aaaaaaaa-0000-4000-8000-000000000003", nil))
	if unauth.Code != 401 {
		t.Fatal("detail not protected", unauth.Code)
	}
	for _, path := range []string{"/api/v1/media/library/managed/not-an-asset", "/api/v1/media/library/managed/aaaaaaaa-0000-4000-8000-000000000099"} {
		if v := invoke(h, "GET", path, "", c.SessionKey); v.Code != 404 {
			t.Fatal(v.Code)
		}
	}
	// Fresh provider classification facts must drive both HTTP surfaces without
	// changing the public detail shape. Fixture below models legacy missing inputs.
	exec(`UPDATE media_items SET metadata=metadata-'countries'-'originalLanguage'`)
	exec(`UPDATE native_tmdb_details SET payload=jsonb_set(payload,'{classification}' ,'{"version":1,"genreIds":[18],"countries":["TW"],"originalLanguage":"zh"}') WHERE tmdb_id=100`)
	refreshed := invoke(h, "GET", "/api/v1/media/library/managed/aaaaaaaa-0000-4000-8000-000000000003", "", c.SessionKey)
	newRaw, e := os.ReadFile("testdata/contracts/t2b-managed-detail.json")
	if e != nil {
		t.Fatal(e)
	}
	var newGot, newWant any
	if refreshed.Code != 200 || json.Unmarshal(refreshed.Body.Bytes(), &newGot) != nil || json.Unmarshal(newRaw, &newWant) != nil {
		t.Fatal(refreshed.Code)
	}
	a, _ := json.Marshal(newGot)
	b, _ := json.Marshal(newWant)
	if string(a) != string(b) {
		t.Fatalf("repaired classification contract differs: %s", a)
	}
	filtered := invoke(h, "GET", "/api/v1/media/library?source=managed&categoryId=r%3Amovie%3Amovie-chinese", "", c.SessionKey)
	var page struct {
		Data struct {
			Total int
			Items []struct{ Category string }
		}
	}
	if filtered.Code != 200 || json.Unmarshal(filtered.Body.Bytes(), &page) != nil || page.Data.Total != 1 || len(page.Data.Items) != 1 || page.Data.Items[0].Category != "电影 / 华语电影" {
		t.Fatal(filtered.Code, filtered.Body.String())
	}

}
