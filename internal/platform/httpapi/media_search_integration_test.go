package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresTMDBCacheSeparatesKindAndConfiguration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	u := os.Getenv("TEST_REDIS_URL")
	if dsn == "" || u == "" {
		t.Skip("isolated DB/Redis required")
	}
	if !strings.HasSuffix(u, "/15") {
		t.Fatal("Redis test DB15")
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
		t.Fatal("test DB")
	}
	db.Exec(ctx, "TRUNCATE integrations CASCADE")
	defer db.Exec(ctx, "TRUNCATE integrations CASCADE")
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	defer func() {
		keys, _, _ := rc.Scan(ctx, 0, "media-tool:tmdb-cache:*", 1000).Result()
		if len(keys) > 0 {
			rc.Del(ctx, keys...)
		}
	}()
	v, _ := secrets.New(strings.Repeat("3", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	profile, err := repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "Owned TMDB", Enabled: true, Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "owned-tmdb-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	c := cfg()
	manager := integrations.NewManager(repo, c)
	defer manager.Close()
	calls := 0
	manager.OutboundHTTP = &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer owned-tmdb-secret" {
			t.Error("missing token")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1,"title":"Owned movie","name":"Owned show","release_date":"2026-01-01","first_air_date":"2026-01-01"}]}`)), Request: r}, nil
	})}
	h := New(c, Dependencies{DB: db, Redis: rc, Integrations: manager, AuthSessions: testSessionReader{}})
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	query := func(kind string) map[string]any {
		r := httptest.NewRequest("GET", "/api/v1/search/media?mediaType="+kind+"&q=Owned%20Cache%20Fixture", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out struct{ Data map[string]any }
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Data
	}
	first := query("movie")
	if first["cached"] != false || calls != 1 {
		t.Fatal(first, calls)
	}
	second := query("movie")
	if second["cached"] != true || calls != 1 {
		t.Fatal(second, calls)
	}
	query("series")
	if calls != 2 {
		t.Fatal("kind cache collision")
	}
	db.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, profile.ID)
	query("movie")
	if calls != 3 {
		t.Fatal("revision cache collision")
	}
	// Subscription/download lookup endpoint shares the same cached result.
	lookup := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/metadata/search?mediaType=movie&q=Owned%20Cache%20Fixture", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w
	}
	if lookup().Header().Get("X-Media-Cache") != "hit" || calls != 3 {
		t.Fatal("cross-entry cache miss")
	}
	prefs, _ := (settings.Repository{DB: db}).Get(ctx)
	original := prefs.Value
	defer func() {
		latest, _ := (settings.Repository{DB: db}).Get(ctx)
		(settings.Repository{DB: db}).Save(ctx, original, latest.Revision)
	}()
	prefs.Value.TMDBCacheSeconds = 45
	saved, e := (settings.Repository{DB: db}).Save(ctx, prefs.Value, prefs.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if lookup().Header().Get("X-Media-Cache") != "miss" || calls != 4 {
		t.Fatal("TTL config not applied")
	}
	keys, _, _ := rc.Scan(ctx, 0, "media-tool:tmdb-cache:*", 1000).Result()
	has45 := false
	for _, key := range keys {
		ttl, _ := rc.TTL(ctx, key).Result()
		if ttl > 40*time.Second && ttl <= 45*time.Second {
			has45 = true
		}
	}
	if !has45 {
		t.Fatal("new TTL absent")
	}
	saved.Value.TMDBCacheSeconds = 0
	if _, e = (settings.Repository{DB: db}).Save(ctx, saved.Value, saved.Revision); e != nil {
		t.Fatal(e)
	}
	lookup()
	lookup()
	if calls != 6 {
		t.Fatal("cache disabled but reused", calls)
	}

}

type mediaCacheTransport func(*http.Request) (*http.Response, error)

func (f mediaCacheTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
