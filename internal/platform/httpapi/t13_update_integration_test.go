package httpapi

import (
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestT13AuthenticatedUpdateStillRequiresCurrentPasswordAndConfirmation(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	c := cfg()
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	name := "t13_" + strings.ReplaceAll(jobs.UUID(), "-", "")[:12]
	if _, e = (accounts.Repository{DB: s.DB}).Register(ctx, name, "owned-pass-123", token); e != nil {
		t.Fatal(e)
	}
	h := New(c, Dependencies{DB: s.DB})
	cases := []struct {
		path, body string
		code       int
	}{
		{"/api/v1/system/online-update", `{"confirm":true}`, 400},
		{"/api/v1/system/online-update", `{"password":"fixture-wrong-123","confirm":false}`, 400},
		{"/api/v1/system/online-update", `{"password":"fixture-wrong-123","confirm":true}`, 403},
		{"/api/v1/system/online-update/control", `{"action":"enable","password":"owned-pass-123","confirm":true}`, 400},
		{"/api/v1/system/online-update/control", `{"action":"confirm_recovery","jobId":"47948a6bb5ac7cd30c0e4b30b4792d9c","password":"fixture-wrong-123","confirm":true}`, 403},
	}
	for _, v := range cases {
		r := httptest.NewRequest("POST", v.path, strings.NewReader(v.body))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		r.Header.Set("X-Requested-With", "media-tool")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != v.code || (strings.Contains(w.Body.String(), "fixture-wrong") || strings.Contains(w.Body.String(), "owned-pass")) {
			t.Fatal(v.path, w.Code, w.Body.String())
		}
	}
	var starts int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action IN ('online_update.queued','online_update.started') AND actor=$1`, name).Scan(&starts); e != nil || starts != 0 {
		t.Fatal("password rejection started update", e, starts)
	}
}
