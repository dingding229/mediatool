package emby

import (
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestProxyPreservesEmbyAuthenticationAndStripsAdminCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items/123" || r.URL.Query().Get("api_key") != "emby-token" {
			t.Errorf("unexpected URL %s", r.URL.String())
		}
		if r.Header.Get("X-Emby-Token") != "emby-token" {
			t.Error("Emby auth lost")
		}
		if _, err := r.Cookie(auth.CookieName); err == nil {
			t.Error("management cookie leaked")
		}
		if _, err := r.Cookie("emby-auth"); err != nil {
			t.Error("Emby cookie lost")
		}
		if r.Header.Get("X-Forwarded-For") == "evil" {
			t.Error("spoofed forwarding header trusted")
		}
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	r := httptest.NewRequest("GET", "/emby/Items/123?api_key=emby-token", nil)
	r.Header.Set("X-Emby-Token", "emby-token")
	r.Header.Set("X-Forwarded-For", "evil")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "admin-secret"})
	r.AddCookie(&http.Cookie{Name: "emby-auth", Value: "keep"})
	w := httptest.NewRecorder()
	NewProxy(target).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
