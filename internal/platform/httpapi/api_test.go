package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func cfg() config.Config {
	return config.Config{LegacyPlaybackEnabled: true, Environment: "development", Address: "127.0.0.1:8080", PublicURL: "https://media.example.com", SessionKey: strings.Repeat("a", 64), STRMKey: strings.Repeat("b", 64), STRMAllowedRoots: []string{"/115/Media"}}
}
func testHandler(resolver storage.PlaybackResolver) (http.Handler, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	policy := playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}
	engine := playback.New(resolver, playback.Options{Policy: policy})
	return New(cfg(), Dependencies{AuthSessions: testSessionReader{}, Storage: resolver, Playback: engine, Logger: slog.New(slog.NewJSONHandler(buf, nil))}), buf
}
func invoke(h http.Handler, method, target, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if key != "" {
		token, _ := (auth.Auth{Key: key}).Issue(time.Now())
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	r.Header.Set("X-Requested-With", "media-tool")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type resolverFunc func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f resolverFunc) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	return f(ctx, ref)
}
func linkTarget() string {
	b := strm.Builder{BaseURL: cfg().PublicURL, APIKey: cfg().STRMKey, AllowedRoots: cfg().STRMAllowedRoots}
	raw, _ := b.Build(storage.FileRef{Path: "/115/Media/电影 & 特别版.mkv"})
	return raw
}
func TestManagementAuthAndCredentialSeparation(t *testing.T) {
	h, _ := testHandler(nil)
	for _, key := range []string{"", cfg().STRMKey} {
		if w := invoke(h, "GET", "/api/v1/system/status", "", key); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := invoke(h, "GET", "/api/v1/system/status", "", cfg().SessionKey)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if strings.Contains(w.Body.String(), cfg().SessionKey) || strings.Contains(w.Body.String(), cfg().STRMKey) {
		t.Fatal("status leaked credentials")
	}
}
func TestDatabaseRequiredAndCSRF(t *testing.T) {
	h, _ := testHandler(nil)
	if w := invoke(h, "POST", "/api/v1/auth/login", `{"username":"admin","password":"synthetic-long-password"}`, ""); w.Code != 503 {
		t.Fatal("fake login without database", w.Code)
	}
	token, _ := (auth.Auth{Key: cfg().SessionKey}).Issue(time.Now())
	r := httptest.NewRequest("POST", "/api/v1/strm/preview", strings.NewReader(`{"path":"/115/Media/a.mkv"}`))
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
}

// Protected business unit tests inject a session fixture, never an HTTP key
// bypass. Account/session persistence is covered by actual PG auth tests.
type testSessionReader struct{}

func (testSessionReader) Session(context.Context, []byte) (accounts.Account, error) {
	return accounts.Account{ID: "unit-fixture", Username: "test_admin", Version: 1}, nil
}

func TestNoFakeBusinessData(t *testing.T) {
	h, _ := testHandler(nil)
	if w := invoke(h, "GET", "/api/v1/sites", "", cfg().SessionKey); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if w := invoke(h, "GET", "/api/v1/jobs", "", cfg().SessionKey); w.Code != 503 {
		t.Fatal(w.Code)
	}
}
func TestPreviewStrictJSON(t *testing.T) {
	h, _ := testHandler(nil)
	for _, body := range []string{`{"path":"/115/Media/a.mkv","unknown":true}`, `{"path":"/115/Media/a.mkv"} {}`, `{"path":"/115/Media/../a.mkv"}`} {
		if w := invoke(h, "POST", "/api/v1/strm/preview", body, cfg().SessionKey); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := invoke(h, "POST", "/api/v1/strm/preview", `{"path":"/115/Media/a.mkv"}`, cfg().SessionKey)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestMissingStorageKeepsOwnerRequestedRawURLLogging(t *testing.T) {
	h, logs := testHandler(nil)
	w := invoke(h, "GET", linkTarget(), "", "")
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), cfg().STRMKey) || !strings.Contains(logs.String(), "apiKey") {
		t.Fatal("private raw request log missing")
	}
}
func TestRedirectAndHEAD(t *testing.T) {
	h, _ := testHandler(resolverFunc(func(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		if ref.Path != "/115/Media/电影 & 特别版.mkv" {
			t.Fatal(ref.Path)
		}
		return storage.DownloadLink{URL: "https://cdn.example.com/video?temporary=1", ExpiresAt: time.Now().Add(time.Minute)}, nil
	}))
	for _, method := range []string{"GET", "HEAD"} {
		w := invoke(h, method, linkTarget(), "", "")
		if w.Code != 302 || w.Header().Get("Location") != "https://cdn.example.com/video?temporary=1" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Header())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD returned body")
		}
	}
}
func TestRejectHeaderRequirementsUnsafeAndExpiredLinks(t *testing.T) {
	for _, tc := range []struct {
		name string
		link storage.DownloadLink
		code int
	}{
		{"headers", storage.DownloadLink{URL: "https://cdn.example.com/video", Headers: map[string]string{"Referer": "private"}}, 409},
		{"ua", storage.DownloadLink{URL: "https://cdn.example.com/video", UserAgent: "required"}, 409},
		{"internal", storage.DownloadLink{URL: "https://127.0.0.1/video"}, 502},
		{"expired", storage.DownloadLink{URL: "https://cdn.example.com/video", ExpiresAt: time.Now().Add(-time.Second)}, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := testHandler(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) { return tc.link, nil }))
			if w := invoke(h, "GET", linkTarget(), "", ""); w.Code != tc.code {
				t.Fatal(w.Code)
			}
		})
	}
}
func TestQueryValidation(t *testing.T) {
	h, _ := testHandler(nil)
	for _, target := range []string{linkTarget() + "&apiKey=duplicate", linkTarget() + "&unknown=true", strm.Endpoint + "?apiKey=" + cfg().STRMKey + "&path=%zz"} {
		w := invoke(h, "GET", target, "", "")
		if w.Code != 400 && w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	u, _ := url.Parse(linkTarget())
	q := u.Query()
	q.Set("apiKey", cfg().SessionKey)
	u.RawQuery = q.Encode()
	if w := invoke(h, "GET", u.String(), "", ""); w.Code != 401 {
		t.Fatal("admin key used as playback key")
	}
}
func TestSafeRedirect(t *testing.T) {
	for _, raw := range []string{"http://example.com/v", "https://user:pass@example.com/v", "https://localhost/v", "https://192.168.1.2/v", "https://example.com/v\r\nX-Evil:1", "javascript:alert(1)"} {
		if _, e := (playback.Policy{}).Syntax(raw); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestInvalidJSONErrorsContainNoBodySecrets(t *testing.T) {
	h, _ := testHandler(nil)
	w := invoke(h, "POST", "/api/v1/strm/preview", `{"secret":"sensitive"}`, cfg().SessionKey)
	var envelope map[string]any
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "sensitive") {
		t.Fatal("body reflected")
	}
}

func TestDisabledLegacyKeyIsRejectedWithoutDatabaseRegistry(t *testing.T) {
	c := cfg()
	c.LegacyPlaybackEnabled = false
	h := New(c, Dependencies{Storage: resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		t.Fatal("disabled legacy resolver invoked")
		return storage.DownloadLink{}, nil
	}), Logger: slog.Default()})
	w := invoke(h, "GET", linkTarget(), "", "")
	if w.Code != 401 {
		t.Fatal("legacy switch bypassed without registry", w.Code)
	}
}

func TestOfficial115AuthorizationEndpointsRequireAccountAndCSRF(t *testing.T) {
	h, _ := testHandler(nil)
	for _, action := range []string{"start", "poll", "cancel"} {
		endpoint := "/api/v1/integrations/00000000-0000-4000-8000-000000000001/115/auth/" + action
		if w := invoke(h, "POST", endpoint, `{}`, ""); w.Code != 401 {
			t.Fatal("unauthenticated authorization allowed", action, w.Code)
		}
		token, _ := (auth.Auth{Key: cfg().SessionKey}).Issue(time.Now())
		r := httptest.NewRequest("POST", endpoint, strings.NewReader(`{}`))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("CSRF authorization accepted", action, w.Code)
		}
	}
}

func TestReadinessRequiresExplicitDevelopmentWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		want      int
	}{
		{"zero", "", http.StatusServiceUnavailable},
		{"production", "production", http.StatusServiceUnavailable},
		{"unknown-is-not-development", "staging", http.StatusServiceUnavailable},
		{"explicit-development", "development", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := cfg()
			c.Environment = tc.env
			h := New(c, Dependencies{})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
			if w.Code != tc.want {
				t.Fatalf("mode %q: status %d, body %s", tc.env, w.Code, w.Body.String())
			}
		})
	}
}

func TestRetiredDataBrowseRoutesAreNotRegistered(t *testing.T) {
	h, _ := testHandler(nil)
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/api/v1/data/postgres/tables", "/api/v1/data/postgres/tables/accounts", "/api/v1/data/redis/keys", "/api/v1/data/redis/value?key=owned"} {
			for _, key := range []string{"", cfg().SessionKey} {
				if w := invoke(h, method, path, "", key); w.Code != http.StatusNotFound {
					t.Fatalf("retired %s %s: %d", method, path, w.Code)
				}
			}
		}
	}
}

func TestRetiredPluginDebugPanelsHaveNoHTTPEntry(t *testing.T) {
	h, _ := testHandler(nil)
	for _, path := range []string{"/api/v1/plugins/owned/tools", "/api/v1/plugins/owned/tools/call", "/api/v1/plugins/owned/auth/status", "/api/v1/plugins/auth/bindings", "/api/v1/plugins/auth/begin", "/api/v1/plugins/owned/state-snapshots"} {
		for _, method := range []string{"GET", "HEAD", "POST"} {
			for _, key := range []string{"", cfg().SessionKey} {
				if w := invoke(h, method, path, "{}", key); w.Code != 404 {
					t.Fatalf("retired %s %s: %d", method, path, w.Code)
				}
			}
		}
	}
}
