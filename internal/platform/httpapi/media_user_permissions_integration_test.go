package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"media-tool/internal/modules/mediausers"
	"media-tool/internal/platform/database"
)

func TestNativeMediaUserPermissionGetPutContractAndNullableUpdates(t *testing.T) {
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
	if err := db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	cleanup := func() {
		t.Helper()
		bounded, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.Exec(bounded, `TRUNCATE administrator_accounts,administrator_auth_limits,media_users,media_user_auth_limits CASCADE`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(bounded, `DELETE FROM native_libraries WHERE id='g:synthetic'`); err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	defer cleanup()
	h := New(cfg(), Dependencies{DB: db})
	call := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		var b bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&b).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("X-Requested-With", "media-tool")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
	w := call("POST", "/api/v1/auth/register", map[string]any{"username": "permission_admin", "password": "admin123", "confirmPassword": "admin123"}, nil)
	expect(w, 201)
	admin := w.Result().Cookies()[0]
	fixture := readPermissionContract(t)
	// A synthetic library uses a stable key; never remove/modify business data.
	var itemID int64
	if err := db.QueryRow(ctx, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('library','g:synthetic') ON CONFLICT(entity_type,internal_key)DO UPDATE SET retired_at=NULL RETURNING id`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type)VALUES('g:synthetic',$1,'synthetic','Synthetic','Synthetic','movies') ON CONFLICT(id)DO NOTHING`, itemID); err != nil {
		t.Fatal(err)
	}
	created, err := (mediausers.Repository{DB: db}).Save(ctx, "", mediausers.Input{Username: fixture.Update.Username, Password: "Synthetic123", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/media-server/users/" + created.ID
	for _, field := range []string{"maxParentalRating", "MaxParentalRating", "BlockUnratedItems"} {
		for _, v := range []any{nil, 65} {
			for _, method := range []string{"POST", "PUT"} {
				target := path
				if method == "POST" {
					target = "/api/v1/media-server/users"
				}
				rejected := call(method, target, map[string]any{"username": created.Username, "revision": 1, field: v}, admin)
				expect(rejected, 400)
				if !bytes.Contains(rejected.Body.Bytes(), []byte(`"invalid_json"`)) {
					t.Fatal(rejected.Body.String())
				}
			}
		}
	}
	unchanged, e := (mediausers.Repository{DB: db}).Get(ctx, created.ID)
	if e != nil || unchanged.Revision != created.Revision {
		t.Fatal("retired-field requests mutated user", e)
	}
	expect(call("GET", path, nil, nil), 401)
	expect(call("PUT", path, fixture.Update, admin), 200)
	w = call("GET", path, nil, admin)
	expect(w, 200)
	if bytes.Contains(w.Body.Bytes(), []byte("maxParentalRating")) {
		t.Fatal("retired response field")
	}
	var got struct {
		Data mediausers.User `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data.MaxStreams != fixture.Response.MaxStreams || (got.Data.ExpiresAt == nil || !got.Data.ExpiresAt.Equal(*fixture.Response.ExpiresAt)) || !reflect.DeepEqual(got.Data.Grants, fixture.Response.Grants) {
		t.Fatal("HTTP permission readback drift", got.Data)
	}
	// Omitted fields preserve restrictions. Explicit null clears them.
	expect(call("PUT", path, map[string]any{"username": created.Username, "enabled": true, "revision": 2, "grants": fixture.Update.Grants}, admin), 200)
	u, err := (mediausers.Repository{DB: db}).Get(ctx, created.ID)
	if err != nil || u.MaxStreams != 3 || u.ExpiresAt == nil {
		t.Fatal("omitted limits were reset", u, err)
	}
	expect(call("PUT", path, map[string]any{"username": created.Username, "enabled": true, "revision": 3, "grants": fixture.Update.Grants, "expiresAt": nil}, admin), 200)
	u, err = (mediausers.Repository{DB: db}).Get(ctx, created.ID)
	if err != nil || u.MaxStreams != 3 || u.ExpiresAt != nil {
		t.Fatal("null did not clear restrictions", u, err)
	}
	expect(call("GET", "/api/v1/media-server/users/aaaaaaaa-0000-4000-8000-000000000000", nil, admin), 404)
	expect(call("PUT", path, map[string]any{"username": created.Username, "revision": 4, "maxStreams": 21}, admin), 400)
	expect(call("PUT", path, map[string]any{"username": created.Username, "revision": 4, "grants": []map[string]any{{"libraryId": "g:synthetic", "canDownload": true, "canPlay": false}}}, admin), 400)
}
