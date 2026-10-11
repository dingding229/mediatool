package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestArchivesAPIAuthenticationCSRFAndInputBounds(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated PG required", err)
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	c := cfg()
	c.STRMOutputDir = t.TempDir()
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
	defer manager.Close()
	h := New(c, Dependencies{AuthSessions: testSessionReader{}, DB: db, Integrations: manager, Logger: slog.Default()})
	id := jobs.UUID()
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/library/archives"}, {"GET", "/api/v1/library/archive-policy"},
		{"POST", "/api/v1/library/archive-policy"}, {"POST", "/api/v1/library/archives/cleanup-preview"},
		{"POST", "/api/v1/library/archives/cleanup-confirm"}, {"GET", "/api/v1/library/archive-cleanups/" + id},
		{"POST", "/api/v1/library/archive-cleanups/" + id + "/retry"},
	}
	for _, route := range routes {
		if w := invoke(h, route.method, route.path, "{}", ""); w.Code != 401 {
			t.Fatal("anonymous archive access", route, w.Code)
		}
		if route.method == "POST" {
			token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
			for _, origin := range []string{"https://evil.example", ""} {
				r := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
				if origin != "" {
					r.Header.Set("Origin", origin)
					r.Header.Set("X-Requested-With", "media-tool")
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 403 {
					t.Fatal("archive CSRF bypass", route, w.Code)
				}
			}
		}
	}
	for _, query := range []string{"page=0", "pageSize=101", "page=1&page=2", "pageSize=", "page=20001", "page=no"} {
		if w := invoke(h, "GET", "/api/v1/library/archives?"+query, "", c.SessionKey); w.Code != 400 {
			t.Fatal("pagination accepted", query, w.Code)
		}
	}
	for _, bad := range []string{"not-uuid", "x' OR true --"} {
		if w := invoke(h, "GET", "/api/v1/library/archive-cleanups/"+strings.ReplaceAll(bad, " ", "%20"), "", c.SessionKey); w.Code != 400 {
			t.Fatal("invalid request UUID", w.Code)
		}
		if w := invoke(h, "POST", "/api/v1/library/archive-cleanups/"+strings.ReplaceAll(bad, " ", "%20")+"/retry", "{}", c.SessionKey); w.Code != 400 {
			t.Fatal("invalid operation UUID", w.Code)
		}
	}
	selections := [][]string{{}, {id, id}, {"not-uuid"}, make([]string, 21)}
	for _, ids := range selections {
		body, _ := json.Marshal(map[string]any{"archiveIds": ids})
		if w := invoke(h, "POST", "/api/v1/library/archives/cleanup-preview", string(body), c.SessionKey); w.Code != 409 {
			t.Fatal("selection bound ignored", w.Code)
		}
	}
	for _, body := range []string{`{"archiveIds":[],"unknown":true}`, `{} {}`} {
		if w := invoke(h, "POST", "/api/v1/library/archives/cleanup-confirm", body, c.SessionKey); w.Code != 400 {
			t.Fatal("invalid JSON accepted", w.Code)
		}
	}
	if w := invoke(h, "POST", "/api/v1/library/archives/cleanup-confirm", `{"requestId":"bad","archiveIds":[],"planDigest":"bad"}`, c.SessionKey); w.Code != 409 {
		t.Fatal("missing consent proof accepted", w.Code)
	}
}
