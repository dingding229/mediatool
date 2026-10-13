package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type ptTransport func(*http.Request) (*http.Response, error)

func (f ptTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostgresPTTemplateAccountValidatesAutoRSSDuplicatesAndEcho(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='hdsky'`)
	defer db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='hdsky'`)
	vault, _ := secrets.New(strings.Repeat("9", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	manager.OutboundHTTP = &http.Client{Transport: ptTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "hdsky.me" || r.Header.Get("User-Agent") != "Owned-UA" || r.Header.Get("Cookie") != "owned-cookie" {
			t.Fatal("identity", r.URL.Host, r.Header.Get("User-Agent"))
		}
		body := `<a href="logout.php">退出</a><a href="userdetails.php?id=42"><b>owned</b></a><a href="/torrentrss.php?passkey=owned">RSS</a>`
		if r.URL.Path == "/torrentrss.php" {
			body = `<rss version="2.0"><channel><title>Owned</title></channel></rss>`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	a := API{Deps: Dependencies{DB: db, Integrations: manager}}
	if e = (sites.Repository{DB: db}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
		t.Fatal(e)
	}
	post := func(id string, revision int64) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"definitionId": "hdsky", "values": map[string]string{"cookie": "owned-cookie", "userAgent": "Owned-UA"}, "revision": revision})
		r := httptest.NewRequest("POST", "/pt/accounts", strings.NewReader(string(raw)))
		if id != "" {
			r.SetPathValue("id", id)
		}
		w := httptest.NewRecorder()
		a.savePTAccount(w, r)
		return w
	}
	w := post("", 0)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			ID       string `json:"id"`
			RSSState string `json:"rssState"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response.Data.RSSState != "ready" {
		t.Fatal(w.Body.String())
	}
	c, e := repo.Get(ctx, response.Data.ID)
	if e != nil || c.Record.Name != "HDSKY" || c.String("endpoint") != "https://hdsky.me" || c.Values["rssUrl"] != "https://hdsky.me/torrentrss.php?passkey=owned" || c.String("userAgent") != "Owned-UA" {
		t.Fatal(c.Record, e)
	}
	if w = post("", 0); w.Code != 409 {
		t.Fatal("duplicate", w.Code)
	}
	echo := httptest.NewRecorder()
	a.ptAccounts(echo, httptest.NewRequest("GET", "/pt/accounts", nil))
	if !strings.Contains(echo.Body.String(), "owned-cookie") || echo.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("echo", echo.Body.String())
	}
	var ciphertext []byte
	db.QueryRow(ctx, "SELECT credentials_ciphertext FROM integrations WHERE id=$1", response.Data.ID).Scan(&ciphertext)
	if strings.Contains(string(ciphertext), "owned-cookie") {
		t.Fatal("unencrypted")
	}
	// Manual RSS/UID must be editable and same-origin validated without replacing good values on failure.
	edit := func(uid, rss string) *httptest.ResponseRecorder {
		latest, _ := repo.Get(ctx, response.Data.ID)
		raw, _ := json.Marshal(map[string]any{"definitionId": "hdsky", "values": map[string]string{"cookie": "owned-cookie", "userAgent": "Owned-UA"}, "revision": latest.Record.Revision, "userId": uid, "rssUrl": rss})
		req := httptest.NewRequest("PUT", "/pt/accounts", strings.NewReader(string(raw)))
		req.SetPathValue("id", response.Data.ID)
		w := httptest.NewRecorder()
		a.savePTAccount(w, req)
		return w
	}
	if w := edit("42", "https://hdsky.me/torrentrss.php?passkey=manual"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	latest, _ := repo.Get(ctx, response.Data.ID)
	if latest.String("rssSource") != "manual" || !strings.Contains(latest.Values["rssUrl"], "manual") {
		t.Fatal("manual override missing")
	}
	for _, sample := range []struct{ uid, rss string }{{"wrong", ""}, {"99", "https://hdsky.me/torrentrss.php?passkey=manual"}, {"42", "https://foreign.invalid/rss?passkey=owned"}} {
		if w := edit(sample.uid, sample.rss); w.Code != 400 {
			t.Fatal("invalid override accepted", w.Code)
		}
	}
	after, _ := repo.Get(ctx, response.Data.ID)
	if after.Record.Revision != latest.Record.Revision || after.Values["rssUrl"] != latest.Values["rssUrl"] {
		t.Fatal("failed manual validation changed good settings")
	}

	if e = (sites.Repository{DB: db}).RegisterGoTemplates(ctx); e != nil {
		t.Fatal(e)
	}
	loaded, e := (sites.Repository{DB: db}).LoadedGoTemplates(ctx)
	if e != nil || !loaded["hdsky"] {
		t.Fatal(loaded, e)
	}
}

func TestPostgresGoRegistrationIsIdempotentAndSiteUniqueAtStorage(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	r := sites.Repository{DB: db}
	if e = r.LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
		t.Fatal(e)
	}
	if e = r.RegisterGoTemplates(ctx); e != nil {
		t.Fatal(e)
	}
	var before, after string
	db.QueryRow(ctx, `SELECT updated_at::text FROM site_definitions WHERE id='hdsky'`).Scan(&before)
	if e = r.RegisterGoTemplates(ctx); e != nil {
		t.Fatal(e)
	}
	db.QueryRow(ctx, `SELECT updated_at::text FROM site_definitions WHERE id='hdsky'`).Scan(&after)
	if before != after {
		t.Fatal("startup changed template snapshot")
	}
	db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='ssd'`)
	defer db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='ssd'`)
	v, _ := secrets.New(strings.Repeat("8", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	input := integrations.Input{Kind: "ptsite", Name: "SSD", Enabled: false, Settings: map[string]any{"definitionId": "ssd", "endpoint": "https://springsunday.net"}}
	if _, e = repo.Save(ctx, "", input); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Save(ctx, "", input); e == nil {
		t.Fatal("duplicate builtin site saved")
	}
}
