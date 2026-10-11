package httpapi

import (
	"context"
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

func TestOutputRecoveryRoutesRequireAdminCSRFAndExplicitPreview(t *testing.T) {
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
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated PG required")
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	c := cfg()
	c.STRMOutputDir = t.TempDir()
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
	defer m.Close()
	h := New(c, Dependencies{DB: db, Integrations: m, AuthSessions: testSessionReader{}, Logger: slog.Default()})
	id := jobs.UUID()
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/transfers/" + id + "/output-preview"}, {"POST", "/api/v1/transfers/" + id + "/resume-import"}} {
		if w := invoke(h, route.method, route.path, "{}", ""); w.Code != 401 {
			t.Fatal("anonymous output recovery", w.Code)
		}
	}
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	r := httptest.NewRequest("POST", "/api/v1/transfers/"+id+"/resume-import", strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	r.Header.Set("Origin", "https://evil.fixture")
	r.Header.Set("X-Requested-With", "media-tool")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF recovery accepted")
	}
	for _, path := range []string{"/api/v1/transfers/bad/output-preview", "/api/v1/transfers/bad/resume-import"} {
		method := "GET"
		if strings.Contains(path, "resume-import") {
			method = "POST"
		}
		if w := invoke(h, method, path, "{}", c.SessionKey); w.Code != 400 {
			t.Fatal("malformed ID accepted", w.Code)
		}
	}
	if w := invoke(h, "POST", "/api/v1/transfers/"+id+"/resume-import", `{"controlRevision":1,"planDigest":"bad"}`, c.SessionKey); w.Code != 400 {
		t.Fatal("missing preview accepted")
	}
	if w := invoke(h, "POST", "/api/v1/transfers/"+id+"/resume-import", `{"controlRevision":1,"planDigest":"bad","override":true}`, c.SessionKey); w.Code != 400 {
		t.Fatal("unknown override accepted")
	}
	for _, query := range []string{"assetId=bad", "assetId=", "assetId=" + id + "&assetId=" + id} {
		if w := invoke(h, "GET", "/api/v1/media/assets?page=1&pageSize=50&"+query, "", c.SessionKey); w.Code != 400 {
			t.Fatal("invalid asset focus accepted", query, w.Code)
		}
	}
}
