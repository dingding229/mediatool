package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/updateidle"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestT14IdleEndpointsAdminOnlyAndPasswordRequired(t *testing.T) {
	h := New(cfg(), Dependencies{})
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/system/online-update/idle"}, {"PUT", "/api/v1/system/online-update/idle"}, {"POST", "/api/v1/system/online-update/idle/check"}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`)))
		if w.Code != 401 {
			t.Fatal(route, w.Code)
		}
	}
	a := &API{}
	for _, body := range []string{`{"enabled":true,"startTime":"23:00","endTime":"05:00","quietMinutes":60}`, `{"enabled":true,"startTime":"23:00","endTime":"05:00","quietMinutes":60,"password":"fixture-sensitive"}`, `{"enabled":false,"startTime":"23:00","endTime":"05:00","quietMinutes":361}`} {
		w := httptest.NewRecorder()
		a.saveIdleUpdate(w, httptest.NewRequest("PUT", "/", strings.NewReader(body)))
		if w.Code != 400 || strings.Contains(w.Body.String(), "fixture-sensitive") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	a.saveIdleUpdate(w, httptest.NewRequest("PUT", "/", strings.NewReader(`{"enabled":true,"startTime":"23:00","endTime":"05:00","quietMinutes":60,"password":"fixture-sensitive","confirm":true}`)))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestT14ShortGateCoversNativeProxyResolverNotAdminHealth(t *testing.T) {
	h := New(cfg(), Dependencies{})
	native, e := NativeMediaHandler(cfg(), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	release, ok := updateidle.Default.CloseFor(time.Minute)
	if !ok {
		t.Fatal("close gate")
	}
	defer release()
	for _, path := range []string{"/emby/Items/1/PlaybackInfo", "/api/v1/cloudStorage/strm302"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "idle_update_admission") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	native.ServeHTTP(w, httptest.NewRequest("POST", "/Items/1/PlaybackInfo", strings.NewReader(`{}`)))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/healthz", "/readyz", "/api/v1/system/online-update/idle"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if strings.Contains(w.Body.String(), "idle_update_admission") {
			t.Fatal("gate blocked", path)
		}
	}
}
func TestT14IssuanceIsDurableAndIgnoresDeniedOrUnrelatedResponses(t *testing.T) {
	// The serial parent owns the DB lock. The native UID65532 HTTP test
	// adapter intentionally does not preserve that parent-only environment flag.
	// Reuse the existing fixture: it requires media_tool_test and a private schema.
	ctx, store := pluginHTTPFixture(t)
	db := store.DB
	if _, e := db.Exec(ctx, `INSERT INTO idle_update_activity VALUES(true,'2000-01-01')`); e != nil {
		t.Fatal(e)
	}
	var e error

	for _, tc := range []struct {
		path    string
		code    int
		changed bool
	}{{"/Items/1/PlaybackInfo", 200, true}, {"/api/v1/cloudStorage/strm302", 302, true}, {"/Items/1/PlaybackInfo", 403, false}, {"/Items/1/PlaybackInfo", 500, false}, {"/Users", 200, false}} {
		if _, e = db.Exec(t.Context(), `UPDATE idle_update_activity SET last_issued_at='2000-01-01'`); e != nil {
			t.Fatal(e)
		}
		h := idleMedia(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code) }), db, func(*http.Request) bool { return true })
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", tc.path, nil))
		var changed bool
		if e = db.QueryRow(t.Context(), `SELECT last_issued_at>'2000-01-02' FROM idle_update_activity`).Scan(&changed); e != nil || changed != tc.changed {
			t.Fatal(tc, changed, e)
		}
	}
}

func TestT14PasswordReauthenticationPersistsOnlyScopedGrantAndNoSecretAudit(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	c := cfg()
	repo := accounts.Repository{DB: store.DB}
	token := legacyAdminCookie(c.SessionKey, time.Now().Add(time.Hour))
	account, e := repo.Register(ctx, "t14_fixture", "synthetic-password", token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.Exec(ctx, `INSERT INTO idle_update_policy(singleton)VALUES(true)`); e != nil {
		t.Fatal(e)
	}
	logs := &bytes.Buffer{}
	h := New(c, Dependencies{DB: store.DB, Logger: slog.New(slog.NewTextHandler(logs, nil))})
	call := func(password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"enabled": true, "startTime": "23:00", "endTime": "05:00", "quietMinutes": 60, "revision": 1, "password": password, "confirm": true})
		r := httptest.NewRequest("PUT", "/api/v1/system/online-update/idle", bytes.NewReader(body))
		r.Header.Set("X-Requested-With", "media-tool")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("wrong-fixture-password"); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("synthetic-password"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var bound bool
	if e = store.DB.QueryRow(ctx, `SELECT enabled AND account_id=$1::uuid AND credential_version=$2 AND grant_id<>'' FROM idle_update_policy WHERE singleton`, account.ID, account.Version).Scan(&bound); e != nil || !bound {
		t.Fatal(bound, e)
	}
	var audits string
	if e = store.DB.QueryRow(ctx, `SELECT coalesce(jsonb_agg(details)::text,'')FROM audit_logs WHERE action='idle_update.configured'`).Scan(&audits); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"synthetic-password", "wrong-fixture-password", token} {
		if strings.Contains(audits, value) || strings.Contains(logs.String(), value) {
			t.Fatal("secret leaked to audit/log")
		}
	}
}
