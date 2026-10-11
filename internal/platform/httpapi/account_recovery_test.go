package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPostgresRecoveryHTTPGates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB required")
	}
	db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE")
	defer db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE")
	c := cfg()
	var logs bytes.Buffer
	h := New(c, Dependencies{DB: db, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	call := func(path, body string, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Origin", c.PublicURL)
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	register := call("/api/v1/auth/register", `{"username":"recovery_admin","password":"old-fixture-2026","confirmPassword":"old-fixture-2026"}`, nil, true)
	if register.Code != 201 {
		t.Fatal(register.Code)
	}
	cookie := register.Result().Cookies()[0]
	if w := call("/api/v1/auth/recovery-code", `{}`, nil, true); w.Code != 401 {
		t.Fatal("unauthenticated recovery issue")
	}
	if w := call("/api/v1/auth/recovery-code", `{"currentPassword":"old-fixture-2026"}`, cookie, false); w.Code != 403 {
		t.Fatal("CSRF issue bypass")
	}
	if w := call("/api/v1/auth/recover", `{}`, nil, false); w.Code != 403 {
		t.Fatal("CSRF reset bypass")
	}
	w := call("/api/v1/auth/recovery-code", `{"currentPassword":"old-fixture-2026"}`, cookie, true)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("issue", w.Code)
	}
	var result struct {
		Data struct {
			Code string `json:"recoveryCode"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Data.Code == "" {
		t.Fatal("code missing")
	}
	body, _ := json.Marshal(map[string]string{"username": "recovery_admin", "recoveryCode": result.Data.Code, "newPassword": "new-fixture-2026", "confirmPassword": "new-fixture-2026"})
	if w = call("/api/v1/auth/recover", string(body), nil, true); w.Code != 200 {
		t.Fatal("recover", w.Code)
	}
	if w = call("/api/v1/auth/recover", string(body), nil, true); w.Code != 400 {
		t.Fatal("replay")
	}
	if w = call("/api/v1/auth/recovery-code", `{"currentPassword":"new-fixture-2026"}`, cookie, true); w.Code != 401 {
		t.Fatal("old session survived")
	}
	for _, secret := range []string{result.Data.Code, "old-fixture-2026", "new-fixture-2026"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("HTTP log leak")
		}
	}
}
