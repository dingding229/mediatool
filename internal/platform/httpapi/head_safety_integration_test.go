package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHEADCannotSaveNetworkProxyWithoutCSRF(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	if _, e := s.DB.Exec(ctx, `INSERT INTO network_proxy_settings(id)VALUES('main')`); e != nil {
		t.Fatal(e)
	}
	repo := networkproxy.Repository{DB: s.DB, Vault: s.Vault}
	before, e := repo.Get(ctx)
	if e != nil {
		t.Fatal(e)
	}
	c := cfg()
	manager := integrations.NewManager(integrations.Repository{DB: s.DB, Vault: s.Vault}, c)
	defer manager.Close()
	h := New(c, Dependencies{DB: s.DB, Integrations: manager, AuthSessions: testSessionReader{}})
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	// A HEAD body must never enter the PUT decoder. Cookie is valid but source
	// is hostile and the CSRF header is absent: only a read may be permitted.
	in := networkproxy.Update{Fields: networkproxy.Fields{Protocol: "http", Host: "synthetic-proxy.invalid", Port: 7890}, Sites: true, Revision: before.Revision}
	body, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		method string
		status int
	}{{http.MethodHead, 200}, {http.MethodPut, 403}} {
		r := httptest.NewRequest(test.method, c.PublicURL+"/api/v1/settings/network-proxy", strings.NewReader(string(body)))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		r.Header.Set("Origin", "https://evil.synthetic.invalid")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("%s status=%d", test.method, w.Code)
		}
		after, e := repo.Get(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if before != after {
			t.Fatalf("%s changed proxy settings without CSRF", test.method)
		}
	}
}

func TestI17StructuredProxyContractDoesNotRevealSecretsOrAcceptLegacyURL(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	if _, e := s.DB.Exec(ctx, `INSERT INTO network_proxy_settings(id) VALUES('main')`); e != nil {
		t.Fatal(e)
	}
	c := cfg()
	manager := integrations.NewManager(integrations.Repository{DB: s.DB, Vault: s.Vault}, c)
	defer manager.Close()
	h := New(c, Dependencies{DB: s.DB, Integrations: manager, AuthSessions: testSessionReader{}})
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, c.PublicURL+"/api/v1/settings/network-proxy", strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		r.Header.Set("Origin", c.PublicURL)
		r.Header.Set("X-Requested-With", "media-tool")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	bad := call("PUT", `{"url":"http://old-secret@proxy:80","revision":1}`)
	if bad.Code != 400 || !strings.Contains(bad.Body.String(), "invalid_json") {
		t.Fatal("legacy URL contract", bad.Code)
	}
	good := call("PUT", `{"protocol":"http","host":"proxy.invalid","port":7890,"username":"owned-user","password":"private-i17","tmdb":false,"sites":false,"revision":1}`)
	if good.Code != 200 {
		t.Fatal("structured save", good.Code, good.Body.String())
	}
	read := call("GET", "")
	if read.Code != 200 || strings.Contains(read.Body.String(), "private-i17") || strings.Contains(read.Body.String(), `"password"`) || strings.Contains(read.Body.String(), `"url"`) || !strings.Contains(read.Body.String(), `"hasPassword":true`) {
		t.Fatal("secret-free response", read.Code)
	}
	conflict := call("PUT", `{"protocol":"http","host":"proxy.invalid","port":7890,"username":"owned-user","revision":1}`)
	if conflict.Code != 409 {
		t.Fatal("stale revision", conflict.Code)
	}
	raw, e := s.Vault.Seal("network-proxy:main", map[string]string{"url": "https://legacy:private-i17@proxy.invalid:443"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE network_proxy_settings SET url_ciphertext=$1,tmdb_enabled=true`, raw); e != nil {
		t.Fatal(e)
	}
	read = call("GET", "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"needsInput":true`) || !strings.Contains(read.Body.String(), `"tmdb":true`) || strings.Contains(read.Body.String(), "private-i17") {
		t.Fatal("legacy shape read", read.Code)
	}
	var after []byte
	if e = s.DB.QueryRow(ctx, `SELECT url_ciphertext FROM network_proxy_settings`).Scan(&after); e != nil || string(after) != string(raw) {
		t.Fatal("legacy ciphertext changed")
	}
}
