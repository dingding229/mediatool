package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/externalapi"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestT15ExternalJWTIPIsAsyncTrustedAndRevocationImmediate(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	signer, _ := externalapi.NewSigner(strings.Repeat("a", 64))
	repo := externalapi.Repository{DB: store.DB, Signer: signer}
	key, e := repo.Create(ctx, "T15 HTTP", []string{"status.read"}, "fixture-admin")
	if e != nil {
		t.Fatal(e)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	rec := externalapi.NewRecorder(store.DB, logger)
	work, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); rec.Run(work) }()
	defer func() { stop(); <-done }()
	h := New(cfg(), Dependencies{DB: store.DB, ExternalSigner: signer, ExternalCalls: rec, Logger: logger})
	call := func(path, peer, xff string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = peer
		r.Header.Set("Authorization", "Bearer "+key.Token)
		r.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("/api/external/v1/status", "8.8.8.8:1234", "1.1.1.1"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ip string
		e = store.DB.QueryRow(ctx, `SELECT coalesce(host(client_ip),'') FROM external_api_calls WHERE key_id=$1 ORDER BY id DESC LIMIT 1`, key.ID).Scan(&ip)
		if e == nil && ip != "" {
			if ip != "8.8.8.8" {
				t.Fatal("spoofed XFF accepted", ip)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("async IP not flushed", e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if w := call("/api/external/v1/downloads", "8.8.8.8:1234", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if e = repo.Revoke(ctx, key.ID, "fixture-admin"); e != nil {
		t.Fatal(e)
	}
	if w := call("/api/external/v1/status", "8.8.8.8:1234", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "/api/external/v1/status?token="+key.Token, nil)
	r.RemoteAddr = "8.8.8.8:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	if strings.Contains(logs.String(), key.Token) {
		t.Fatal("JWT entered logs")
	}
}
func TestT15ManagementRevealRejectsMediaBearerAndRequiresCSRF(t *testing.T) {
	h := New(cfg(), Dependencies{})
	for _, route := range []struct{ method, path string }{{"POST", "/api/v1/external-api/keys/11111111-2222-4333-8444-555555555555/reveal"}, {"DELETE", "/api/v1/external-api/keys/11111111-2222-4333-8444-555555555555"}} {
		r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"action":"view"}`))
		r.Header.Set("Authorization", "Bearer fake.jwt.value")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(route, w.Code)
		}
	}
}
func TestT15CredentialRedactionIsScopedAndDoesNotEraseBusinessURLs(t *testing.T) {
	raw := "http://owned.invalid/api/external/v1/status?token=mt_api_" + strings.Repeat("a", 64) + "&filter=movie"
	out := externalLogURL(raw)
	if strings.Contains(out, strings.Repeat("a", 64)) || !strings.Contains(out, "filter=movie") {
		t.Fatal("credential redaction mismatch")
	}
	normal := "https://owned.invalid/api/external/v1/status?z=%20&a=1"
	if externalLogURL(normal) != normal {
		t.Fatal("non-secret original URL changed")
	}
	u := "https://cdn.115cdn.net/owned?sign=retained-business-url"
	if externalLogText(u) != u {
		t.Fatal("business URL changed")
	}
}
func TestT15SummaryDropsAllSecretStorageColumns(t *testing.T) {
	_, store := pluginHTTPFixture(t)
	var n int
	if e := store.DB.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema()AND table_name='external_api_keys' AND column_name IN('token_ciphertext','token_digest','prefix','expires_at')`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	raw, _ := json.Marshal(externalapi.KeySummary{})
	if strings.Contains(string(raw), `"token"`) {
		t.Fatal("summary contains bearer")
	}
}

func TestT15HTTPRepeatedDisclosureIsAuditedAndFailClosed(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	c := cfg()
	signer, _ := externalapi.NewSigner(strings.Repeat("a", 64))
	session := legacyAdminCookie(c.SessionKey, time.Now().Add(time.Hour))
	if _, e := (accounts.Repository{DB: store.DB}).Register(ctx, "t15_admin", "synthetic-password", session); e != nil {
		t.Fatal(e)
	}
	h := New(c, Dependencies{DB: store.DB, ExternalSigner: signer})
	repo := externalapi.Repository{DB: store.DB, Signer: signer}
	key, e := repo.Create(ctx, "T15 disclosure", []string{"status.read"}, "t15_admin")
	if e != nil {
		t.Fatal(e)
	}
	call := func(action string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/external-api/keys/"+key.ID+"/reveal", strings.NewReader(`{"action":"`+action+`"}`))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("view", false); w.Code != 403 {
		t.Fatal("CSRF not enforced", w.Code)
	}
	for range 2 {
		w := call("view", true)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
		var v struct {
			Data externalapi.Key `json:"data"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &v); e != nil || v.Data.Token != key.Token {
			t.Fatal("HTTP re-sign mismatch", e)
		}
	}
	var count int
	var audits string
	if e = store.DB.QueryRow(ctx, `SELECT count(*),coalesce(jsonb_agg(details)::text,'') FROM audit_logs WHERE action='external_api.revealed.view' AND details->>'id'=$1`, key.ID).Scan(&count, &audits); e != nil || count != 2 || strings.Contains(audits, key.Token) {
		t.Fatal(count, e)
	}
	if _, e = store.DB.Exec(ctx, `ALTER TABLE audit_logs ADD CONSTRAINT t15_reject_copy CHECK(action<>'external_api.revealed.copy')`); e != nil {
		t.Fatal(e)
	}
	w := call("copy", true)
	if w.Code != 503 || strings.Contains(w.Body.String(), key.Token) {
		t.Fatal("failed audit disclosed bearer", w.Code)
	}
}
