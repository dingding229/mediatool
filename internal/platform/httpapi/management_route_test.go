package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestManagementEmbyReloadCannotAccidentallyNavigateToPlaybackServer(t *testing.T) {
	var rootCalls, apiCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Real-Emby", "fixture")
		if r.URL.Path == "/" {
			rootCalls.Add(1)
			w.Header().Set("Location", "/web/index.html")
			w.WriteHeader(302)
			return
		}
		apiCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Version":"owned-fixture"}`))
	}))
	defer upstream.Close()
	c := cfg()
	c.EmbyUpstream = upstream.URL
	c.EmbyProxyEnabled = true
	c.EmbyPublicURL = "https://gateway.test"
	handler := New(c, Dependencies{})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "https://management.test/emby", nil))
	if w.Code != 200 || w.Header().Get("Location") != "" || !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "Media Tool") || rootCalls.Load() != 0 {
		t.Fatal("SPA reload proxied to Emby", w.Code)
	}
	w = httptest.NewRecorder()
	EmbyHandler(handler).ServeHTTP(w, httptest.NewRequest("GET", "https://gateway.test/", nil))
	if w.Code != 302 || rootCalls.Load() != 1 {
		t.Fatal("dedicated Emby root no longer proxied", w.Code)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Path != "/web/index.html" {
		t.Fatal("gateway redirect broken", err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "https://management.test/emby/System/Info/Public", nil))
	if w.Code != 200 || w.Header().Get("X-Real-Emby") != "fixture" || apiCalls.Load() != 1 {
		t.Fatal("legacy API mount lost")
	}
}
