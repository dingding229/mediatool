package httpapi

import (
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTelegramActionProvisioningIsAdminCSRFOnlyAndAbsentOnMediaPort(t *testing.T) {
	c := cfg()
	h := New(c, Dependencies{AuthSessions: testSessionReader{}})
	path := "/api/v1/messaging/command-actions"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	media, e := NativeMediaHandler(c, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	media.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
	if w.Code == 200 || w.Code == 202 {
		t.Fatal("admin ingress on media port")
	}
}
