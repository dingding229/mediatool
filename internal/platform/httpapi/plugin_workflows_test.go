package httpapi

import (
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkflowAndAuthAPIsRequireAdminCSRFAndKeepPasswordLogin(t *testing.T) {
	c := cfg()
	h := New(c, Dependencies{AuthSessions: testSessionReader{}})
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/plugins/11111111-0000-4000-8000-000000000062/tools/call", "/api/v1/plugins/agent/chat", "/api/v1/plugins/workflows", "/api/v1/plugins/auth/bindings", "/api/v1/plugins/auth/begin"} {
		for _, session := range []bool{false, true} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			if session {
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expected := 401
			if session {
				expected = 403
			}
			if w.Code != expected {
				t.Fatal(path, session, w.Code)
			}
		}
	}
	// The broker has no registered public callback/session issuer; built-in login remains its own handler.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`)))
	if w.Code == 404 || w.Code == 200 {
		t.Fatal("local login boundary", w.Code)
	}
}
