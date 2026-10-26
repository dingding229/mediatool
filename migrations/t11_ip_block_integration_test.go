package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"media-tool/internal/modules/ipblock"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestT11MigrationUpDownUpOnlyOwnTables(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 86); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO application_settings DEFAULT VALUES ON CONFLICT(singleton) DO UPDATE SET revision=application_settings.revision`)
	f.exec(t, `INSERT INTO player_ip_block_rules(id,network,note) VALUES('11000000-0000-4000-8000-000000000001','203.0.113.9','测试')`)
	if _, e := f.provider.Down(f.ctx); e != nil {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('player_ip_block_rules','player_ip_block_policy')`); n != 0 {
		t.Fatal(n)
	}
	if n := f.integer(t, `SELECT count(*) FROM application_settings`); n != 1 {
		t.Fatal("unrelated settings changed", n)
	}
	if _, e := f.provider.UpTo(f.ctx, 86); e != nil {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM player_ip_block_rules`); n != 0 {
		t.Fatal(n)
	}
	if n := f.integer(t, `SELECT revision FROM player_ip_block_policy WHERE singleton`); n != 1 {
		t.Fatal(n)
	}
}
func TestT11SharedLiveCacheAdminPreviewMediaRoutesTrustAndCounterDeletion(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 86); e != nil {
		t.Fatal(e)
	}
	db := catalogTestPool(t, f)
	defer db.Close()
	ctx := f.ctx
	f.exec(t, `INSERT INTO application_settings DEFAULT VALUES ON CONFLICT(singleton) DO UPDATE SET revision=application_settings.revision`)
	repo := ipblock.Repository{DB: db}
	gate := ipblock.New(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e := gate.Reload(ctx); e != nil {
		t.Fatal(e)
	}
	c := config.Config{Environment: "development", SessionKey: strings.Repeat("t", 64), PublicURL: "https://admin.synthetic.invalid"}
	deps := httpapi.Dependencies{DB: db, IPBlock: gate, AuthSessions: h3Admin{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	main := httpapi.New(c, deps)
	native, e := httpapi.NativeMediaHandler(c, deps)
	if e != nil {
		t.Fatal(e)
	}
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	call := func(h http.Handler, method, path, peer, xff string, body any, admin, csrf bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.RemoteAddr = peer
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		if admin {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	assert := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	path := "/api/v1/player-access/ip-blocklist"
	assert(call(main, "POST", path, "8.8.8.8:1234", "", map[string]any{"network": "203.0.113.9", "revision": 1, "confirm": true}, false, true), 401)
	assert(call(main, "POST", path, "8.8.8.8:1234", "", map[string]any{"network": "203.0.113.9", "revision": 1, "confirm": true}, true, false), 403)
	assert(call(main, "POST", path, "8.8.8.8:1234", "", map[string]any{"network": "203.0.113.9", "revision": 1, "confirm": true}, true, true), 200)
	assert(call(main, "POST", path, "8.8.8.8:1234", "", map[string]any{"network": "1.1.1.1", "revision": 1, "confirm": true}, true, true), 409)
	assert(call(main, "POST", path+"/check", "8.8.8.8:1234", "", map[string]any{"address": "203.0.113.9"}, false, true), 401)
	assert(call(main, "POST", path+"/check", "8.8.8.8:1234", "", map[string]any{"address": "203.0.113.9"}, true, false), 403)
	w := call(main, "POST", path+"/check", "8.8.8.8:1234", "", map[string]any{"address": "203.0.113.9"}, true, true)
	assert(w, 200)
	if !strings.Contains(w.Body.String(), `"matched":true`) {
		t.Fatal(w.Body.String())
	}
	if n := f.integer(t, `SELECT hit_count FROM player_ip_block_rules`); n != 0 {
		t.Fatal("preview counted", n)
	}
	protectedBefore, e := maintenance.Protected(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	assert(call(main, "GET", "/emby/System/Info/Public", "203.0.113.9:1234", "", nil, false, false), 404) // T16: retired proxy, not a native protocol alias on this listener.
	for _, p := range []string{strm.Endpoint} {
		assert(call(main, "GET", p, "203.0.113.9:1234", "8.8.8.8", nil, false, false), 403)
	}
	for _, c := range []struct{ method, path string }{{"GET", "/System/Info/Public"}, {"POST", "/Users/AuthenticateByName"}, {"GET", "/Items/100001"}, {"GET", "/Items/100001/Images/Primary"}, {"POST", "/Items/100001/PlaybackInfo"}, {"GET", "/Videos/100001/stream.mkv"}, {"GET", "/emby/System/Info/Public"}, {"GET", "/mediabrowser/System/Info/Public"}} {
		assert(call(native, c.method, c.path, "203.0.113.9:1234", "1.1.1.1", nil, false, false), 403)
	}
	// Cover every documented native route/method using the real database-backed shared gate.
	rawDoc, e := os.ReadFile("../api/openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Listener string `json:"x-listener"`
		} `json:"paths"`
	}
	if e = json.Unmarshal(rawDoc, &doc); e != nil {
		t.Fatal(e)
	}
	extraNative := 0
	parameters := regexp.MustCompile(`\{[^}]+\}`)
	for path, methods := range doc.Paths {
		for method, operation := range methods {
			if operation.Listener != "native-media" {
				continue
			}
			concrete := strings.NewReplacer("{ext}", "mkv", "{format}", "vtt", "{type}", "Primary", "{index}", "0").Replace(path)
			concrete = parameters.ReplaceAllString(concrete, "100001")
			assert(call(native, strings.ToUpper(method), concrete, "203.0.113.9:1234", "", nil, false, false), 403)
			extraNative++
		}
	}
	if extraNative < 100 {
		t.Fatal("native route inventory incomplete", extraNative)
	}
	// These are not in the media gate, even when the transport peer is blacklisted.
	assert(call(main, "GET", path, "203.0.113.9:1234", "", nil, true, false), 200)
	assert(call(main, "GET", "/healthz", "203.0.113.9:1234", "", nil, false, false), 200)
	readyBefore := call(main, "GET", "/readyz", "1.1.1.1:1234", "", nil, false, false).Code
	assert(call(main, "GET", "/readyz", "203.0.113.9:1234", "", nil, false, false), readyBefore)
	assert(call(main, "GET", "/api/v1/system/version", "203.0.113.9:1234", "", nil, true, false), 200)
	assert(call(main, "GET", strm.Endpoint, "1.1.1.1:1234", "203.0.113.9", nil, false, false), 401) // untrusted XFF cannot manufacture a match
	f.exec(t, `UPDATE application_settings SET value=jsonb_set(value,'{trustedProxyCidrs}','["10.0.0.0/8"]'),revision=revision+1`)
	assert(call(main, "GET", strm.Endpoint, "10.0.0.2:1234", "203.0.113.9", nil, false, false), 403)
	assert(call(main, "GET", strm.Endpoint, "10.0.0.2:1234", "bad,203.0.113.9", nil, false, false), 503)
	if e := gate.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	protectedAfter, e := maintenance.Protected(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	if protectedBefore["player_ip_block_rules"] != protectedAfter["player_ip_block_rules"] {
		t.Fatal("statistics invalidated policy protection")
	}
	if n := f.integer(t, `SELECT hit_count FROM player_ip_block_rules`); n != int64(10+extraNative) {
		t.Fatal("exactly live requests counted", n)
	}
	p, e := repo.Load(ctx)
	if e != nil {
		t.Fatal(e)
	}
	id := p.Rules[0].ID
	batch := map[string]ipblock.Delta{id: {Count: 3, Last: time.Now()}}
	for i := 0; i < 2; i++ {
		if e := repo.AddHits(ctx, "11000000-0000-4000-8000-000000000098", batch); e != nil {
			t.Fatal(e)
		}
	}
	if n := f.integer(t, `SELECT hit_count FROM player_ip_block_rules`); n != int64(13+extraNative) {
		t.Fatal("same SQL batch double counted", n)
	}
	_, _ = gate.Evaluate(netip.MustParseAddr("203.0.113.9"), true)
	assert(call(main, "DELETE", path+"/"+id, "8.8.8.8:1234", "", map[string]any{"revision": p.Revision, "confirm": true}, true, true), 200)
	if e = gate.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if e = repo.AddHits(ctx, "11000000-0000-4000-8000-000000000099", map[string]ipblock.Delta{id: {Count: 10, Last: time.Now()}}); e != nil {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM player_ip_block_rules`); n != 0 {
		t.Fatal("deleted rule resurrected", n)
	}
	assert(call(main, "GET", strm.Endpoint, "203.0.113.9:1234", "", nil, false, false), 401)
	// No accidental self-block without a second explicit acknowledgement.
	w = call(main, "POST", path, "8.8.8.8:1234", "", map[string]any{"network": "8.8.8.0/24", "revision": 3, "confirm": true}, true, true)
	assert(w, 400)
	if !strings.Contains(w.Body.String(), "player_ip_self_confirmation") {
		t.Fatal(w.Body.String())
	}
	// Initial database policy load failure recovers without restarting either handler.
	f.exec(t, `DELETE FROM player_ip_block_policy`)
	unavailable := ipblock.New(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e = unavailable.Reload(ctx); e == nil {
		t.Fatal("expected load failure")
	}
	failedMain := httpapi.New(c, httpapi.Dependencies{DB: db, IPBlock: unavailable, AuthSessions: h3Admin{}})
	assert(call(failedMain, "GET", strm.Endpoint, "1.1.1.1:1234", "", nil, false, false), 503)
	assert(call(failedMain, "GET", "/healthz", "1.1.1.1:1234", "", nil, false, false), 200)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- unavailable.Run(runCtx) }()
	defer func() { stop(); <-done }()
	f.exec(t, `INSERT INTO player_ip_block_policy(singleton) VALUES(true)`)
	deadline := time.Now().Add(4 * time.Second)
	for !unavailable.Status().Available && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !unavailable.Status().Available {
		t.Fatal("automatic recovery failed")
	}
	assert(call(failedMain, "GET", strm.Endpoint, "1.1.1.1:1234", "", nil, false, false), 401)
}
func TestT11RepositoryLimitCIDRIPv6ConflictAndAudit(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 86); e != nil {
		t.Fatal(e)
	}
	db := catalogTestPool(t, f)
	defer db.Close()
	r := ipblock.Repository{DB: db}
	for i, network := range []string{"8.8.8.0/24", "2001:db8::/32"} {
		p, e := r.Change(f.ctx, ipblock.Mutation{Network: network, Revision: int64(i + 1), Confirm: true})
		if e != nil {
			t.Fatal(e)
		}
		s := ipblock.New(r, nil)
		if e = s.Reload(f.ctx); e != nil {
			t.Fatal(e)
		}
		ip := "8.8.8.7"
		if i == 1 {
			ip = "2001:db8::123"
		}
		m, e := s.Evaluate(netip.MustParseAddr(ip), false)
		if e != nil || !m.Matched || m.Revision != p.Revision {
			t.Fatal(m, e)
		}
	}
	gate := ipblock.New(r, nil)
	if e := gate.Reload(f.ctx); e != nil {
		t.Fatal(e)
	}
	native, e := httpapi.NativeMediaHandler(config.Config{}, httpapi.Dependencies{DB: db, IPBlock: gate})
	if e != nil {
		t.Fatal(e)
	}
	ipv6Request := httptest.NewRequest("GET", "/System/Info/Public", nil)
	ipv6Request.RemoteAddr = "[2001:db8::9]:1234"
	ipv6Response := httptest.NewRecorder()
	native.ServeHTTP(ipv6Response, ipv6Request)
	if ipv6Response.Code != 403 {
		t.Fatal("IPv6 HTTP bypass", ipv6Response.Code)
	}
	main := httpapi.New(config.Config{Environment: "development"}, httpapi.Dependencies{DB: db, IPBlock: gate})
	cidrRequest := httptest.NewRequest("GET", strm.Endpoint, nil)
	cidrRequest.RemoteAddr = "8.8.8.7:1234"
	cidrResponse := httptest.NewRecorder()
	main.ServeHTTP(cidrResponse, cidrRequest)
	if cidrResponse.Code != 403 {
		t.Fatal("CIDR HTTP bypass", cidrResponse.Code)
	}
	p, e := r.Load(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e = r.Change(f.ctx, ipblock.Mutation{ID: p.Rules[0].ID, Network: "1.1.1.1", Note: "中文备注", Revision: p.Revision, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='player_ip.rules_changed' AND details ? 'id'`); n != 3 {
		t.Fatal(n)
	}
	f.exec(t, `DELETE FROM player_ip_block_rules`)
	f.exec(t, `INSERT INTO player_ip_block_rules(id,network) SELECT md5(i::text)::uuid,('10.'||(i/256)||'.'||(i%256)||'.1/32')::cidr FROM generate_series(0,9999)i`)
	if _, e = r.Change(f.ctx, ipblock.Mutation{Network: "203.0.113.9", Revision: p.Revision, Confirm: true}); !errors.Is(e, ipblock.ErrLimit) {
		t.Fatal(fmt.Sprint(e))
	}
	c := config.Config{Environment: "development", SessionKey: strings.Repeat("l", 64), PublicURL: "https://admin.synthetic.invalid"}
	handler := httpapi.New(c, httpapi.Dependencies{DB: db, AuthSessions: h3Admin{}})
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	request := httptest.NewRequest("POST", "/api/v1/player-access/ip-blocklist", strings.NewReader(fmt.Sprintf(`{"network":"203.0.113.9","revision":%d,"confirm":true}`, p.Revision)))
	request.RemoteAddr = "8.8.8.8:1234"
	request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	request.Header.Set("X-Requested-With", "media-tool")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "10000") {
		t.Fatal("API capacity error unclear", response.Code, response.Body.String())
	}

}

type t11Resolver func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f t11Resolver) Resolve(ctx context.Context, r storage.FileRef) (storage.DownloadLink, error) {
	return f(ctx, r)
}
func TestT11BothReal302ChainsStay302ExceptWhileMatched(t *testing.T) {
	f := newPlaybackFixture(t, 86)
	ctx := f.ctx
	gate := ipblock.New(ipblock.Repository{DB: f.pool}, nil)
	if e := gate.Reload(ctx); e != nil {
		t.Fatal(e)
	}
	c := config.Config{Environment: "development", SessionKey: "synthetic-native-playback-auth", STRMKey: "synthetic-global-strm-key-3132", PublicURL: "https://resolver.synthetic.invalid", MemberPublicURL: "https://native.synthetic.invalid", STRMAllowedRoots: []string{"/115"}, LegacyPlaybackEnabled: true}
	calls := 0
	source := t11Resolver(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		calls++
		return storage.DownloadLink{URL: "https://cdn.example.test/t11", CacheUntil: time.Now().Add(time.Minute)}, nil
	})
	engine := playback.New(source, playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	main := httpapi.New(c, httpapi.Dependencies{DB: f.pool, IPBlock: gate, Storage: source, Playback: engine})
	native, e := httpapi.NativeMediaHandler(c, httpapi.Dependencies{DB: f.pool, IPBlock: gate, Integrations: f.manager, NativePlayback: f.service})
	if e != nil {
		t.Fatal(e)
	}
	link, e := (strm.Builder{BaseURL: c.PublicURL, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}).Build(storage.FileRef{Path: "/115/T11/owned.mkv"})
	if e != nil {
		t.Fatal(e)
	}
	target, _ := url.Parse(link)
	call := func(h http.Handler, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "8.8.8.8:1234"
		r.Header.Set("User-Agent", "Infuse/8")
		r.Header.Set("X-Emby-Token", f.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	video := "/Videos/" + f.item + "/stream.mkv?MediaSourceId=" + f.a.asset
	for _, c := range []struct {
		h    http.Handler
		path string
	}{{main, target.RequestURI()}, {native, video}} {
		w := call(c.h, c.path)
		if w.Code != 302 || w.Header().Get("Location") == "" {
			t.Fatal("unmatched 302 broken", w.Code, w.Body.String())
		}
	}
	p, e := gate.Change(ctx, ipblock.Mutation{Network: "8.8.8.8", Revision: 1, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	oldCalls, oldDownloads := calls, f.downloads.Load()
	for _, c := range []struct {
		h    http.Handler
		path string
	}{{main, target.RequestURI()}, {native, video}} {
		w := call(c.h, c.path)
		if w.Code != 403 || w.Header().Get("Location") != "" {
			t.Fatal("matched path leaked redirect", w.Code)
		}
	}
	if calls != oldCalls || f.downloads.Load() != oldDownloads {
		t.Fatal("provider invoked after IP denial")
	}
	if _, e = gate.Change(ctx, ipblock.Mutation{ID: p.Rules[0].ID, Delete: true, Revision: p.Revision, Confirm: true}); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		h    http.Handler
		path string
	}{{main, target.RequestURI()}, {native, video}} {
		w := call(c.h, c.path)
		if w.Code != 302 {
			t.Fatal("delete did not immediately restore 302", w.Code, w.Body.String())
		}
	}
}
