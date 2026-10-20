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
	in := networkproxy.Record{URL: "http://synthetic-proxy.invalid:7890", Sites: true, Revision: before.Revision}
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
