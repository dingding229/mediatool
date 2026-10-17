package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediausers"
	"media-tool/internal/platform/database"
	"os"
)

func TestNativeMediaUsersAdminManagementKeepsCredentialsPrivate(t *testing.T) {
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
	h := New(cfg(), Dependencies{DB: db})
	call := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
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
			t.Fatalf("want %d got %d %s", status, w.Code, w.Body.String())
		}
	}
	w := call("POST", "/api/v1/auth/register", map[string]any{"username": "media_admin", "password": "admin123", "confirmPassword": "admin123"}, nil)
	expect(w, 201)
	admin := w.Result().Cookies()[0]
	expect(call("GET", "/api/v1/media-server/providers", nil, admin), 200)
	w = call("POST", "/api/v1/media-server/users", map[string]any{"username": "media_user", "password": "first123", "enabled": true, "grants": []map[string]any{}}, admin)
	expect(w, 201)
	var result struct {
		Data mediausers.User `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.MustChangePassword != false || strings.Contains(w.Body.String(), "first123") || strings.Contains(w.Body.String(), "hash") {
		t.Fatal("unsafe user projection", w.Body.String())
	}
	expect(call("POST", "/api/v1/media-server/users", map[string]any{"username": "media_user", "password": "first123", "enabled": true}, admin), 409)
	expect(call("POST", "/api/v1/media-server/users", map[string]any{"username": "bad", "password": "first123", "enabled": true, "grants": []map[string]any{{"libraryId": "x"}, {"libraryId": "x"}}}, admin), 400)
	expect(call("PUT", "/api/v1/media-server/users/"+result.Data.ID, map[string]any{"username": "media_user", "displayName": "Owned Media", "enabled": true, "revision": result.Data.Revision, "grants": []any{}}, admin), 200)
	expect(call("POST", "/api/v1/media-server/users/"+result.Data.ID+"/sessions/revoke", map[string]any{"revision": 2}, admin), 200)
	expect(call("POST", "/api/v1/media-server/users/"+result.Data.ID+"/password", map[string]any{"revision": 3, "password": "reset123", "mustChangePassword": true}, admin), 200)
	var requireChange bool
	if err := db.QueryRow(ctx, `SELECT must_change_password FROM media_users WHERE id=$1`, result.Data.ID).Scan(&requireChange); err != nil || !requireChange {
		t.Fatal("administrator reset policy was not persisted", err)
	}
	expect(call("DELETE", "/api/v1/media-server/users/"+result.Data.ID, map[string]any{"revision": 4}, admin), 200)
}
