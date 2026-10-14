package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNativeMediaUsersIsolationPermissionsAndRevocation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	cleanup := `TRUNCATE administrator_accounts,administrator_auth_limits,media_users,media_user_auth_limits,storage_connections,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	if _, err = db.Exec(ctx, `INSERT INTO native_media_libraries(id,name,media_type)VALUES('movies','电影','movie'),('series','剧集','series')ON CONFLICT(id)DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	h := New(cfg(), Dependencies{DB: db})
	call := func(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("X-Requested-With", "media-tool")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d %s", status, w.Code, w.Body.String())
		}
	}
	w := call("POST", "/api/v1/auth/register", map[string]any{"username": "media_admin", "password": "admin123", "confirmPassword": "admin123"})
	expect(w, 201)
	admin := w.Result().Cookies()[0]
	w = call("GET", "/api/v1/media-server/providers", nil, admin)
	expect(w, 200)
	if !strings.Contains(w.Body.String(), `"playbackTickets":false`) {
		t.Fatal("advertised unimplemented playback")
	}
	w = call("POST", "/api/v1/media-server/users", map[string]any{"username": "member_one", "password": "first123", "enabled": true, "grants": []map[string]any{{"libraryId": "movies", "canPlay": true}}}, admin)
	expect(w, 201)
	var result struct {
		Data mediausers.User `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	u := result.Data
	if !u.MustChangePassword || u.Revision != 1 || strings.Contains(w.Body.String(), "hash") || strings.Contains(w.Body.String(), "first123") {
		t.Fatal("user projection or policy", w.Body.String())
	}
	expect(call("POST", "/api/v1/media-server/users", map[string]any{"username": "member_one", "password": "first123", "enabled": true}, admin), 409)
	expect(call("POST", "/api/v1/media-server/users", map[string]any{"username": "member_bad", "password": "first123", "enabled": true, "grants": []map[string]any{{"libraryId": "movies"}, {"libraryId": "movies"}}}, admin), 400)
	login := func(name, password string) *http.Cookie {
		t.Helper()
		reply := call("POST", "/api/v1/member/auth/login", map[string]any{"username": name, "password": password})
		expect(reply, 200)
		cookie := reply.Result().Cookies()[0]
		if cookie.Name != mediausers.CookieName || cookie.Path != "/api/v1/member" || !cookie.HttpOnly {
			t.Fatal("realm cookie")
		}
		return cookie
	}
	member := login("member_one", "first123")
	// Neither administrator nor member sessions can cross the realm boundary,
	// even when a client manually renames or forwards the other cookie.
	for _, path := range []string{"/api/v1/player-access/policy", "/api/v1/player-access/observations", "/api/v1/media-server/users", "/api/v1/integrations", "/api/v1/logs", "/api/v1/data/postgres/tables", "/api/v1/system/status"} {
		expect(call("GET", path, nil, member), 401)
	}
	expect(call("GET", "/api/v1/system/status", nil, &http.Cookie{Name: auth.CookieName, Value: member.Value}), 401)
	expect(call("GET", "/api/v1/member/auth/session", nil, admin), 401)
	expect(call("GET", "/api/v1/member/auth/session", nil, &http.Cookie{Name: mediausers.CookieName, Value: admin.Value}), 401)
	expect(call("POST", "/api/v1/member/auth/register", map[string]string{}, member), 404)
	expect(call("POST", "/api/v1/member/auth/login", map[string]string{"username": "media_admin", "password": "admin123"}), 401)
	expect(call("GET", "/api/v1/member/libraries", nil, member), 403)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 200)
	r := httptest.NewRequest("POST", "/api/v1/member/auth/login", strings.NewReader(`{"username":"member_one","password":"first123"}`))
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("X-Requested-With", "media-tool")
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, r)
	expect(bad, 403)
	r = httptest.NewRequest("POST", "/api/v1/member/auth/password", strings.NewReader(`{}`))
	r.AddCookie(member)
	bad = httptest.NewRecorder()
	h.ServeHTTP(bad, r)
	expect(bad, 403)
	expect(call("POST", "/api/v1/member/auth/password", map[string]any{"currentPassword": "first123", "newPassword": "next12", "confirmPassword": "different"}, member), 400)
	expect(call("POST", "/api/v1/member/auth/password", map[string]any{"currentPassword": "first123", "newPassword": "abc12", "confirmPassword": "abc12"}, member), 400)
	expect(call("POST", "/api/v1/member/auth/password", map[string]any{"currentPassword": "wrong123", "newPassword": "next123", "confirmPassword": "next123"}, member), 401)
	expect(call("POST", "/api/v1/member/auth/password", map[string]any{"currentPassword": "first123", "newPassword": "next123", "confirmPassword": "next123"}, member), 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 401)
	member = login("member_one", "next123")
	w = call("GET", "/api/v1/member/libraries", nil, member)
	expect(w, 200)
	if !strings.Contains(w.Body.String(), "movies") || strings.Contains(w.Body.String(), "series") {
		t.Fatal("library grant leak")
	}
	expect(call("GET", "/api/v1/member/libraries/series/items", nil, member), 403)
	// Assets are synthetic fixtures in media_tool_test, never real media writes.
	_, err = db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES('aaaaaaaa-0000-0000-0000-000000000001','Owned fixture','clouddrive2');
 INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES('aaaaaaaa-0000-0000-0000-000000000002','movie',123,'Owned Movie','{"overview":"Owned synopsis","posterPath":"/owned.jpg"}');
 INSERT INTO cloud_files(id,storage_id,full_path,pick_code,size_bytes,state)VALUES
 ('aaaaaaaa-0000-0000-0000-000000000003','aaaaaaaa-0000-0000-0000-000000000001','/PRIVATE-MEDIA/movie.mkv','PRIVATE-PICKCODE',1,'present'),
 ('aaaaaaaa-0000-0000-0000-000000000004','aaaaaaaa-0000-0000-0000-000000000001','/PRIVATE-MEDIA/trash.mkv','PRIVATE-PICKCODE',1,'trashed'),
 ('aaaaaaaa-0000-0000-0000-000000000005','aaaaaaaa-0000-0000-0000-000000000001','/PRIVATE-MEDIA/unconfirmed.mkv','PRIVATE-PICKCODE',1,'present');
 INSERT INTO media_assets(id,cloud_file_id)VALUES
 ('aaaaaaaa-0000-0000-0000-000000000006','aaaaaaaa-0000-0000-0000-000000000003'),
 ('aaaaaaaa-0000-0000-0000-000000000007','aaaaaaaa-0000-0000-0000-000000000004'),
 ('aaaaaaaa-0000-0000-0000-000000000008','aaaaaaaa-0000-0000-0000-000000000005');
 INSERT INTO media_asset_bindings(asset_id,media_id,state)VALUES
 ('aaaaaaaa-0000-0000-0000-000000000006','aaaaaaaa-0000-0000-0000-000000000002','ready'),
 ('aaaaaaaa-0000-0000-0000-000000000007','aaaaaaaa-0000-0000-0000-000000000002','ready'),
 ('aaaaaaaa-0000-0000-0000-000000000008','aaaaaaaa-0000-0000-0000-000000000002','pending');`)
	if err != nil {
		t.Fatal(err)
	}
	w = call("GET", "/api/v1/member/libraries/movies/items?pageSize=24", nil, member)
	expect(w, 200)
	if !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"title":"Owned Movie"`) || !strings.Contains(w.Body.String(), "/api/v1/member/libraries/movies/items/") || strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), "filePath") {
		t.Fatal("unsafe catalog projection", w.Body.String())
	}
	expect(call("GET", "/api/v1/member/libraries/movies/items?page=0", nil, member), 400)
	expect(call("GET", "/api/v1/member/libraries/movies/items/aaaaaaaa-0000-0000-0000-000000000007/poster", nil, member), 404)
	expect(call("POST", "/api/v1/member/libraries/movies/items/aaaaaaaa-0000-0000-0000-000000000006/playback", map[string]any{}, member), 501)
	expect(call("POST", "/api/v1/member/libraries/movies/items/aaaaaaaa-0000-0000-0000-000000000007/playback", map[string]any{}, member), 404)
	// A second media user starts with no implicit access to the first user's scope.
	expect(call("POST", "/api/v1/media-server/users", map[string]any{"username": "member_two", "password": "second123", "enabled": true}, admin), 201)
	second := login("member_two", "second123")
	expect(call("POST", "/api/v1/member/auth/password", map[string]any{"currentPassword": "second123", "newPassword": "third123", "confirmPassword": "third123"}, second), 200)
	second = login("member_two", "third123")
	w = call("GET", "/api/v1/member/libraries", nil, second)
	expect(w, 200)
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatal("new user implicitly granted")
	}
	expect(call("GET", "/api/v1/member/libraries/movies/items", nil, second), 403)
	// Edit races, permission removal, disabled users and reset passwords invalidate
	// previously issued sessions, rather than relying on stale browser state.
	w = call("PUT", "/api/v1/media-server/users/"+u.ID, map[string]any{"username": "member_one", "enabled": true, "revision": 2, "grants": []any{}}, admin)
	expect(w, 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 401)
	expect(call("PUT", "/api/v1/media-server/users/"+u.ID, map[string]any{"username": "member_one", "enabled": true, "revision": 2}, admin), 409)
	member = login("member_one", "next123")
	expect(call("GET", "/api/v1/member/libraries/movies/items", nil, member), 403)
	expect(call("POST", "/api/v1/media-server/users/"+u.ID+"/sessions/revoke", map[string]any{"revision": 3}, admin), 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 401)
	member = login("member_one", "next123")
	expect(call("POST", "/api/v1/media-server/users/"+u.ID+"/password", map[string]any{"revision": 4, "password": "reset123"}, admin), 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 401)
	member = login("member_one", "reset123")
	expect(call("GET", "/api/v1/member/libraries", nil, member), 403)
	expect(call("PUT", "/api/v1/media-server/users/"+u.ID, map[string]any{"username": "member_one", "enabled": false, "revision": 5}, admin), 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, member), 401)
	expect(call("POST", "/api/v1/member/auth/login", map[string]any{"username": "member_one", "password": "reset123"}), 401)
	expect(call("DELETE", "/api/v1/media-server/users/"+u.ID, map[string]any{"revision": 6}, admin), 200)
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM media_user_sessions WHERE user_id=$1`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatal("deleted user's sessions remain")
	}
	expect(call("POST", "/api/v1/member/auth/logout", map[string]any{}, second), 200)
	expect(call("GET", "/api/v1/member/auth/session", nil, second), 401)
	expect(call("GET", "/api/v1/auth/session", nil, admin), 200)
	// Dedicated member throttles don't consume administrator login counters.
	repo := mediausers.Repository{DB: db}
	for i := 0; i < 3; i++ {
		allowed, err := repo.TakeAttempt(ctx, cfg().SessionKey, "owned-rate-fixture", 2)
		if err != nil || allowed != (i < 2) {
			t.Fatal("member limiter", allowed, err)
		}
	}
}
