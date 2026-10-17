package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/linkcache"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/database"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresUAAdminWorkflowCachedSTRMAndUnavailableDenial(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	redisURL := os.Getenv("TEST_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("isolated PG and Redis required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated DB only")
	}
	rc, err := cache.Open(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	reset := `TRUNCATE player_ua_rules,player_ua_observations; INSERT INTO player_ua_policy(id)VALUES('main')ON CONFLICT(id)DO UPDATE SET enabled=false,revision=1`
	if _, err = db.Exec(ctx, reset); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), reset)
	var calls atomic.Int32
	ua := "Owned-Player/" + time.Now().Format("150405.000000000")
	source := resolverFunc(func(_ context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		if ref.UserAgent != ua {
			t.Error("actual UA replaced", ref.UserAgent)
		}
		calls.Add(1)
		return storage.DownloadLink{URL: "https://cdn.example.test/owned", CacheUntil: time.Now().Add(time.Minute)}, nil
	})
	cached := &linkcache.Cached{Source: source, Redis: rc, Version: func(context.Context, storage.FileRef) (string, error) { return "ua-isolated", nil }}
	engine := playback.New(cached, playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	h := New(cfg(), Dependencies{DB: db, Storage: source, Playback: engine, AuthSessions: testSessionReader{}})
	token, _ := (auth.Auth{Key: cfg().SessionKey}).Issue(time.Now())
	admin := &http.Cookie{Name: auth.CookieName, Value: token}
	call := func(method, path, useragent string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("User-Agent", useragent)
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
			t.Fatal(w.Code, w.Body.String())
		}
	}
	expect(call("GET", "/api/v1/player-access/policy", "", nil, nil), 401)
	expect(call("GET", linkTarget(), ua, nil, nil), 302)
	expect(call("HEAD", linkTarget(), ua, nil, nil), 302)
	if calls.Load() != 1 {
		t.Fatal("fixture not cached", calls.Load())
	}
	w := call("GET", "/api/v1/player-access/observations", "", nil, admin)
	expect(w, 200)
	var list struct {
		Data playeraccess.Page `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Data.Items) != 1 || list.Data.Items[0].UserAgent != ua || list.Data.Items[0].RequestCount != 2 {
		t.Fatal(err, w.Body.String())
	}
	id := list.Data.Items[0].ID
	expect(call("POST", "/api/v1/player-access/observations/"+id+"/allow", "", map[string]any{"revision": 1, "label": "Owned", "confirm": false}, admin), 400)
	w = call("POST", "/api/v1/player-access/observations/"+id+"/allow", "", map[string]any{"revision": 1, "label": "Owned", "confirm": true}, admin)
	expect(w, 200)
	var policy struct {
		Data playeraccess.Policy `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &policy)
	p := policy.Data
	p.Enabled = true
	r := httptest.NewRequest("PUT", "/api/v1/player-access/policy", strings.NewReader(`{}`))
	r.AddCookie(admin)
	r.Header.Set("Origin", "https://untrusted.invalid")
	r.Header.Set("X-Requested-With", "media-tool")
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, r)
	expect(bad, 403)
	w = call("PUT", "/api/v1/player-access/policy", "", p, admin)
	expect(w, 200)
	json.Unmarshal(w.Body.Bytes(), &policy)
	p = policy.Data
	expect(call("GET", linkTarget(), ua, nil, nil), 302)
	if calls.Load() != 1 {
		t.Fatal("policy cache check wrong")
	}
	for _, method := range []string{"GET", "HEAD"} {
		w = call(method, linkTarget(), "UnknownPlayer/1", nil, nil)
		expect(w, 403)
		if w.Header().Get("Location") != "" {
			t.Fatal("denial leaked CDN")
		}
	}
	// Matching UA still needs the permanent file credential; no auth bypass.
	expect(call("GET", strings.Replace(linkTarget(), cfg().STRMKey, "incorrect", 1), ua, nil, nil), 401)
	// Replace the allowed rule without invalidating Redis: old UA must be denied
	// before the cached URL can be read or returned.
	p.Rules = []playeraccess.Rule{{MatchType: "prefix", UserAgent: "OtherPlayer/", Enabled: true}}
	w = call("PUT", "/api/v1/player-access/policy", "", p, admin)
	expect(w, 200)
	json.Unmarshal(w.Body.Bytes(), &policy)
	p = policy.Data
	w = call("GET", linkTarget(), ua, nil, nil)
	expect(w, 403)
	if calls.Load() != 1 {
		t.Fatal("denied cache request reached resolver")
	}
	// Header aliases/secondary public listener cannot evade the same policy.
	root := EmbyHandler(h)
	r = httptest.NewRequest("POST", "/Items/11/PlaybackInfo", strings.NewReader(`{}`))
	r.Header.Set("User-Agent", ua)
	bad = httptest.NewRecorder()
	root.ServeHTTP(bad, r)
	expect(bad, 403)
	expect(call("GET", "/api/v1/player-access/observations?q=Owned-Player&filter=unlisted", "", nil, admin), 200)
	// An unavailable policy/aggregate write is not permission to return a URL.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	// Playback deliberately does not take the administrator advisory lock.
	// A table lock simulates actual policy storage unavailability instead.
	if _, err = tx.Exec(ctx, `LOCK TABLE player_ua_policy IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	w = call("GET", linkTarget(), ua, nil, nil)
	expect(w, 503)
	if w.Header().Get("Location") != "" {
		t.Fatal("unavailable policy CDN")
	}
	tx.Rollback(ctx)
	// Concurrent edits share one revision; exactly one may commit.
	repo := playeraccess.Repository{DB: db}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Save(ctx, p); results <- err }()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if err == playeraccess.ErrConflict {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("lost revision fence")
	}
	p, _ = repo.Policy(ctx)
	expect(call("POST", "/api/v1/player-access/observations/clear", "", map[string]any{"revision": p.Revision, "confirm": true}, admin), 200)
	current, _ := repo.Policy(ctx)
	if !current.Enabled || len(current.Rules) != 1 {
		t.Fatal("clear changed access policy")
	}
	cached.Invalidate(ctx, storage.FileRef{Path: "/115/Media/电影 & 特别版.mkv", UserAgent: ua, ClientIP: "192.0.2.1"})
}
