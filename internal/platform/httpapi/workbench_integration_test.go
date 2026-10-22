package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/ownerrecovery"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPostgresWorkbenchMandatoryResetAndReadOnlyTools(t *testing.T) {
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
	c.SecretKey = strings.Repeat("1", 64)
	vault, _ := secrets.New(c.SecretKey)
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
	defer manager.Close()
	store, err := logging.Open(t.TempDir(), "debug")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logger, _ := logging.NewControlled(store, "debug", &store.Level)
	h := New(c, Dependencies{DB: db, Integrations: manager, Logs: store, Logger: logger})
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", c.PublicURL)
		r.Header.Set("X-Requested-With", "media-tool")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	registered := call("POST", "/api/v1/auth/register", `{"username":"reset_admin","password":"Original-2026","confirmPassword":"Original-2026"}`, nil)
	if registered.Code != 201 {
		t.Fatal(registered.Code)
	}
	oldCookie := registered.Result().Cookies()[0]
	marker := filepath.Join(t.TempDir(), "password")
	os.WriteFile(marker, nil, 0600)
	var console bytes.Buffer
	if _, err = ownerrecovery.Apply(ctx, db, marker, &console); err != nil {
		t.Fatal(err)
	}
	tmp := regexp.MustCompile(`Temporary password: ([A-Za-z0-9_-]{20})`).FindStringSubmatch(console.String())[1]
	body, _ := json.Marshal(map[string]string{"username": "reset_admin", "password": tmp})
	login := call("POST", "/api/v1/auth/login", string(body), nil)
	if login.Code != 200 || !strings.Contains(login.Body.String(), `"mustChangePassword":true`) {
		t.Fatal("temp login", login.Code)
	}
	cookie := login.Result().Cookies()[0]
	for _, path := range []string{"/api/v1/settings/application", "/api/v1/logs", "/api/v1/logs/query", "/api/v1/integrations", "/api/v1/files/roots"} {
		if w := call("GET", path, "", cookie); w.Code != 403 {
			t.Fatal("mandatory reset bypass", path, w.Code)
		}
	}
	if w := call("GET", "/api/v1/auth/session", "", oldCookie); w.Code != 401 {
		t.Fatal("old session active")
	}
	changed, _ := json.Marshal(map[string]string{"currentPassword": tmp, "newPassword": "Updated-2026", "confirmPassword": "Updated-2026"})
	if w := call("POST", "/api/v1/auth/password", string(changed), cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]string{"username": "reset_admin", "password": "Updated-2026"})
	login = call("POST", "/api/v1/auth/login", string(body), nil)
	cookie = login.Result().Cookies()[0]
	for _, path := range []string{"/api/v1/settings/application", "/api/v1/logs/summary", "/api/v1/logs/query", "/api/v1/plugins"} {
		if w := call("GET", path, "", cookie); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/v1/logs/clear", `{"confirm":false}`, cookie); w.Code != 400 {
		t.Fatal("unconfirmed clear")
	}
	logger.Warn("owned warning")
	if w := call("GET", "/api/v1/logs?level=warn", "", cookie); w.Code != 200 || !strings.Contains(w.Body.String(), "owned warning") {
		t.Fatal("log filter")
	}

	if w := call("GET", "/api/v1/logs/query?level=warn&keyword=owned&pageSize=50&page=1", "", cookie); w.Code != 200 || !strings.Contains(w.Body.String(), "owned warning") || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatal("query filters", w.Code, w.Body.String())
	}
	for _, query := range []string{"page=0", "pageSize=201", "from=bad", "from=2026-10-12T00:00:00Z&to=2026-10-11T00:00:00Z", "before=2999-01-01T00:00:00Z", "level=invalid"} {
		if w := call("GET", "/api/v1/logs/query?"+query, "", cookie); w.Code != 400 {
			t.Fatal("invalid log query", query, w.Code)
		}
	}
	if w := call("GET", "/api/v1/logs/query", "", nil); w.Code != 401 {
		t.Fatal("anonymous query", w.Code)
	}
	if w := call("POST", "/api/v1/auth/recover", `{}`, nil); w.Code != 404 {
		t.Fatal("legacy recovery still active")
	}
	if w := call("POST", "/api/v1/auth/recovery-code", `{}`, cookie); w.Code != 404 {
		t.Fatal("legacy recovery still active")
	}
	parsed := call("POST", "/api/v1/media/recognize-name", `{"name":"Movie.2026.1080p.WEB-DL.x265-TEAM.mkv"}`, cookie)
	if parsed.Code != 200 || !strings.Contains(parsed.Body.String(), `"year":"2026"`) {
		t.Fatal("parse route")
	}
	rows, _ := store.Read("", 200)
	all, _ := json.Marshal(rows)
	if strings.Contains(string(all), tmp) {
		t.Fatal("temporary password reached web log")
	}
}
