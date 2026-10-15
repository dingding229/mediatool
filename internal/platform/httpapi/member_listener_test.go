package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemberListenerBlocksAdministrativeSurface(t *testing.T) {
	h, _ := testHandler(nil)
	member := MemberHandler(h)
	for _, p := range []string{"/api/v1/auth/login", "/api/v1/settings/access", "/api/v1/downloads/manual", "/api/v1/media-server/users", "/emby/Users", "/logs", "/watch", "/assets/../api/v1/settings/access"} {
		r := httptest.NewRequest("GET", p, nil)
		w := httptest.NewRecorder()
		member.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("%s %d", p, w.Code)
		}
	}
	for _, p := range []string{"/", "/login", "/password"} {
		r := httptest.NewRequest("GET", p, nil)
		w := httptest.NewRecorder()
		member.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `name="media-tool-realm" content="member"`) {
			t.Fatalf("member root failed %s %d", p, w.Code)
		}
	}
	manifestWriter := httptest.NewRecorder()
	member.ServeHTTP(manifestWriter, httptest.NewRequest("GET", "/manifest.webmanifest", nil))
	if manifestWriter.Code != 200 || !strings.Contains(manifestWriter.Body.String(), `"start_url":"/"`) {
		t.Fatal("member PWA points at management route", manifestWriter.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/v1/member/auth/session", nil)
	w := httptest.NewRecorder()
	member.ServeHTTP(w, r)
	if w.Code == http.StatusOK {
		t.Fatal("anonymous member authenticated")
	}
}
