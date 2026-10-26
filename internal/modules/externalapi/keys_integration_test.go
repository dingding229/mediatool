package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func keyFixture(t *testing.T) Repository {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	base, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(base.Close)
	name := "jwt_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, e = base.Exec(t.Context(), `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := base.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); e != nil {
			t.Error(e)
		}
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	db, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	for _, table := range []string{"external_api_keys", "external_api_calls", "audit_logs"} {
		if _, e = db.Exec(t.Context(), `CREATE TABLE `+table+`(LIKE public.`+table+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	signer, e := NewSigner(strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	return Repository{DB: db, Signer: signer}
}
func TestPermanentScopedAPIKeysRevokeAndLimit(t *testing.T) {
	r := keyFixture(t)
	ctx := t.Context()
	k, e := r.Create(ctx, "Owned API", []string{"status.read"}, "fixture-admin")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(k.Token, ".") != 2 {
		t.Fatal("JWT missing")
	}
	for range 2 {
		v, e := r.Reveal(ctx, k.ID, "fixture-admin", "view")
		if e != nil || v.Token != k.Token {
			t.Fatal("reveal changed token", e)
		}
	}
	fresh, e := NewSigner(strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	r.Signer = fresh
	v, e := r.Reveal(ctx, k.ID, "fixture-admin", "copy")
	if e != nil || v.Token != k.Token {
		t.Fatal("restart changed token", e)
	}
	list, e := (Repository{DB: r.DB}).List(ctx)
	if e != nil || len(list) != 1 {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(list)
	for _, bad := range []string{k.Token, `"token"`, "ciphertext", "digest", "expiresAt"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("list leaked credential material")
		}
	}
	if _, _, e = r.Admit(ctx, k.Token, "downloads.run", "POST", "/download"); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
	for range 120 {
		if _, _, e = r.Admit(ctx, k.Token, "status.read", "GET", "/status"); e != nil {
			t.Fatal(e)
		}
	}
	if _, _, e = r.Admit(ctx, k.Token, "status.read", "GET", "/status"); !errors.Is(e, ErrRate) {
		t.Fatal(e)
	}
	if e = r.Delete(ctx, k.ID, "fixture-admin"); !errors.Is(e, ErrDelete) {
		t.Fatal(e)
	}
	if e = r.Revoke(ctx, k.ID, "fixture-admin"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = r.Admit(ctx, k.Token, "status.read", "GET", "/status"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal(e)
	}
	if _, e = r.Reveal(ctx, k.ID, "fixture-admin", "view"); !errors.Is(e, ErrRevoked) {
		t.Fatal(e)
	}
	if e = r.Delete(ctx, k.ID, "fixture-admin"); e != nil {
		t.Fatal(e)
	}
	var calls int
	r.DB.QueryRow(ctx, `SELECT count(*)FROM external_api_calls`).Scan(&calls)
	if calls != 0 {
		t.Fatal("temporary call rows not cleaned")
	}
	var audits int
	r.DB.QueryRow(ctx, `SELECT count(*)FROM audit_logs WHERE action='external_api.revealed.view'`).Scan(&audits)
	if audits != 2 {
		t.Fatal(audits)
	}
}
func TestRevealAuditFailureNeverDiscloses(t *testing.T) {
	r := keyFixture(t)
	ctx := t.Context()
	k, e := r.Create(ctx, "Audit fixture", []string{"status.read"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.DB.Exec(ctx, `ALTER TABLE audit_logs ADD CONSTRAINT reject_reveal CHECK(action<>'external_api.revealed.view')`); e != nil {
		t.Fatal(e)
	}
	v, e := r.Reveal(ctx, k.ID, "fixture-admin", "view")
	if e == nil || v.Token != "" {
		t.Fatal("audit failure disclosed token")
	}
}
func TestAsyncObservationOrderingRetentionAndDeleteNeverRecreates(t *testing.T) {
	r := keyFixture(t)
	ctx := t.Context()
	k, e := r.Create(ctx, "IP fixture", []string{"status.read"})
	if e != nil {
		t.Fatal(e)
	}
	_, first, e := r.Admit(ctx, k.Token, "status.read", "GET", "/status")
	if e != nil {
		t.Fatal(e)
	}
	_, last, e := r.Admit(ctx, k.Token, "status.read", "GET", "/status")
	if e != nil {
		t.Fatal(e)
	}
	rec := NewRecorder(r.DB, nil)
	items := []Observation{{k.ID, last, "2001:4860:4860::8888", "trusted_proxy", 200}, {k.ID, first, "8.8.8.8", "direct", 200}}
	if e = rec.Flush(ctx, items); e != nil {
		t.Fatal(e)
	}
	list, e := r.List(ctx)
	if e != nil || list[0].LastIP != "2001:4860:4860::8888" {
		t.Fatal(list, e)
	}
	calls, e := r.Calls(ctx, k.ID, 50)
	if e != nil || len(calls) != 2 || calls[0].ClientIP == "" {
		t.Fatal(calls, e)
	}
	if _, e = r.DB.Exec(ctx, `UPDATE external_api_calls SET created_at=now()-interval '31 days';UPDATE external_api_keys SET last_ip_at=now()-interval '31 days'`); e != nil {
		t.Fatal(e)
	}
	if e = rec.Prune(ctx); e != nil {
		t.Fatal(e)
	}
	calls, _ = r.Calls(ctx, k.ID, 50)
	list, _ = r.List(ctx)
	if len(calls) != 0 || list[0].LastIP != "" {
		t.Fatal("retention failed")
	}
	if e = r.Revoke(ctx, k.ID); e != nil {
		t.Fatal(e)
	}
	if e = r.Delete(ctx, k.ID); e != nil {
		t.Fatal(e)
	}
	if e = rec.Flush(ctx, items); e != nil {
		t.Fatal(e)
	}
	list, _ = r.List(ctx)
	if len(list) != 0 {
		t.Fatal("late observation recreated credential")
	}
	start := time.Now()
	for range 5000 {
		rec.Submit(items[0])
	}
	if time.Since(start) > time.Second {
		t.Fatal("IP submit blocked")
	}
}

func TestT15OperatorProbeCleansOnlyItsTemporaryCredential(t *testing.T) {
	r := keyFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rec := NewRecorder(r.DB, nil)
	done := make(chan struct{})
	go func() { defer close(done); rec.Run(ctx) }()
	defer func() { cancel(); <-done }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		token := strings.TrimPrefix(q.Header.Get("Authorization"), "Bearer ")
		id, call, e := r.Admit(q.Context(), token, "status.read", q.Method, q.URL.Path)
		if e != nil {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
		rec.Submit(Observation{KeyID: id, CallID: call, IP: "127.0.0.1", Source: "direct", Status: 200})
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")
	out, e := VerifyLocalLifecycle(ctx, r.DB, strings.Repeat("a", 64), addr)
	if e != nil || !out.SameToken || out.ViewAudits != 2 || out.ReadStatus != 200 || out.ClientIP != "127.0.0.1" || out.RevokedStatus != 401 || !out.Deleted {
		t.Fatal(out, e)
	}
}
