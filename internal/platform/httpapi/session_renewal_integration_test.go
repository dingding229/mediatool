package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/ownerrecovery"
)

func legacyAdminCookie(key string, expiry time.Time) string {
	data := strconv.FormatInt(expiry.Unix(), 10) + "." + strings.Repeat("a", 48)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("media-tool:administrator-session:v2:" + data))
	return data + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func t1AdminCall(h http.Handler, token, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = "8.8.8.8:1234"
	r.Header.Set("User-Agent", "T1/1")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func t1Expiry(t *testing.T, ctx context.Context, db *pgxpool.Pool, digest []byte) (time.Time, time.Time) {
	t.Helper()
	var created, expiry time.Time
	if e := db.QueryRow(ctx, `SELECT created_at,expires_at FROM administrator_sessions WHERE token_digest=$1`, digest).Scan(&created, &expiry); e != nil {
		t.Fatal(e)
	}
	return created, expiry
}
func assertT1Cookie(t *testing.T, w *httptest.ResponseRecorder, expected time.Time, token string) {
	t.Helper()
	cookies := w.Result().Cookies()
	if strings.Contains(w.Body.String(), token) {
		t.Fatal("session credential exposed in JSON")
	}
	if len(cookies) != 1 {
		t.Fatal("expected exactly one refreshed cookie", len(cookies))
	}
	c := cookies[0]
	if c.Name != auth.CookieName || c.Value != token || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatal("cookie identity/security attributes changed")
	}
	if !c.Expires.Equal(expected.UTC().Truncate(time.Second)) || time.Duration(c.MaxAge)*time.Second-time.Until(expected) > 2*time.Second || time.Duration(c.MaxAge)*time.Second-time.Until(expected) < -time.Second {
		t.Fatal("cookie expiration differs from PG")
	}
}
func TestT1LegacyEightHourTokenRenewalAndReadOnlySSEChecks(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: s.DB}
	c := cfg()
	// Signed nominal expiry has already passed, but a previously renewed PG row
	// is live. Upgrades must not kick out such a legacy session.
	token := legacyAdminCookie(c.SessionKey, time.Now().Add(-time.Hour))
	sum := sha256.Sum256([]byte(token))
	if (auth.Auth{Key: c.SessionKey}).Verify(token, time.Now()) || !(auth.Auth{Key: c.SessionKey}).VerifySignature(token) {
		t.Fatal("legacy fixture")
	}
	if _, e := repo.Register(ctx, "t1_legacy", "synthetic-password", token, accounts.SessionMetadata{IP: "8.8.8.8", UserAgent: "T1/1"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `UPDATE administrator_sessions SET created_at=now()-interval '9 hours',last_active_at=now(),expires_at=now()+interval '6 days 22 hours' WHERE token_digest=$1`, sum[:]); e != nil {
		t.Fatal(e)
	}
	beforeCreated, _ := t1Expiry(t, ctx, s.DB, sum[:])
	h := New(c, Dependencies{DB: s.DB})
	w := t1AdminCall(h, token, "/api/v1/auth/session")
	if w.Code != 200 {
		t.Fatal("legacy session rejected", w.Code)
	}
	created, expiry := t1Expiry(t, ctx, s.DB, sum[:])
	var databaseNow time.Time
	if e := s.DB.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); e != nil {
		t.Fatal(e)
	}
	if !created.Equal(beforeCreated) || expiry.Sub(databaseNow) < auth.Lifetime-5*time.Second || expiry.Sub(databaseNow) > auth.Lifetime {
		t.Fatal("renewal window/creation", "createdSame", created.Equal(beforeCreated), "remaining", time.Until(expiry))
	}
	assertT1Cookie(t, w, expiry, token)
	if v := t1AdminCall(h, token, "/api/v1/auth/session"); v.Code != 200 || len(v.Result().Cookies()) != 0 {
		t.Fatal("renewal not throttled", v.Code)
	}
	if _, e := s.DB.Exec(ctx, `UPDATE administrator_sessions SET expires_at=now()+interval '6 days 22 hours' WHERE token_digest=$1`, sum[:]); e != nil {
		t.Fatal(e)
	}
	_, before := t1Expiry(t, ctx, s.DB, sum[:])
	api := &API{Config: c, Deps: Dependencies{DB: s.DB}, Auth: auth.Auth{Key: c.SessionKey}}
	r := httptest.NewRequest("GET", "/api/v1/jobs/events", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	if !api.authorized(r) {
		t.Fatal("stream authorization failed")
	}
	_, after := t1Expiry(t, ctx, s.DB, sum[:])
	if !before.Equal(after) {
		t.Fatal("SSE/nested authorization performed DB-only renewal")
	}
	// Restore's recent-login gate remains tied to creation, despite fresh activity.
	r.SetPathValue("id", "11111111-0000-4000-8000-000000000001")
	wr := httptest.NewRecorder()
	if api.recentRestoreSession(wr, r) || wr.Code != 403 {
		t.Fatal("renewal made old login recent")
	}
	for name, sql := range map[string]string{"idle expired": `UPDATE administrator_sessions SET expires_at=now()-interval '1 second'`, "absolute cap": `UPDATE administrator_sessions SET created_at=now()-interval '30 days',expires_at=now()+interval '7 days'`} {
		if _, e := s.DB.Exec(ctx, sql); e != nil {
			t.Fatal(e)
		}
		if v := t1AdminCall(h, token, "/api/v1/auth/session"); v.Code != 401 || len(v.Result().Cookies()) != 0 {
			t.Fatal(name, v.Code)
		}
	}
}
func TestT1InitialLoginCookieAndThirtyDayCap(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: s.DB}
	c := cfg()
	seed, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if _, e := repo.Register(ctx, "t1_login", "synthetic-password", seed); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"t1_login","password":"synthetic-password"}`))
	r.Header.Set("X-Requested-With", "media-tool")
	w := httptest.NewRecorder()
	h := New(c, Dependencies{DB: s.DB})
	h.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("login failed", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	sum := sha256.Sum256([]byte(cookie.Value))
	created, expiry := t1Expiry(t, ctx, s.DB, sum[:])
	if expiry.Sub(created) != auth.Lifetime || cookie.MaxAge != int(auth.Lifetime.Seconds()) || expiry.After(created.Add(auth.AbsoluteLifetime)) {
		t.Fatal("new login not initialized to 7d", "dbLifetime", expiry.Sub(created), "cookieMaxAge", cookie.MaxAge)
	}
	assertT1Cookie(t, w, expiry, cookie.Value)
	if _, e := s.DB.Exec(ctx, `UPDATE administrator_sessions SET created_at=now()-interval '29 days',expires_at=now()+interval '1 hour',last_active_at=now() WHERE token_digest=$1`, sum[:]); e != nil {
		t.Fatal(e)
	}
	w = t1AdminCall(h, cookie.Value, "/api/v1/auth/session")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	created, expiry = t1Expiry(t, ctx, s.DB, sum[:])
	if !expiry.Equal(created.Add(auth.AbsoluteLifetime)) {
		t.Fatal("30d cap not used")
	}
	assertT1Cookie(t, w, expiry, cookie.Value)
}
func TestT1InvalidationNeverRenewed(t *testing.T) {
	for _, kind := range []string{"revoke", "password", "administrator reset", "owner recovery", "temporary expired"} {
		t.Run(kind, func(t *testing.T) {
			ctx, s := pluginHTTPFixture(t)
			repo := accounts.Repository{DB: s.DB}
			c := cfg()
			token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
			sum := sha256.Sum256([]byte(token))
			a, e := repo.Register(ctx, "t1_invalidate", "synthetic-password", token)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.DB.Exec(ctx, `UPDATE administrator_sessions SET expires_at=now()+interval '1 hour' WHERE token_digest=$1`, sum[:]); e != nil {
				t.Fatal(e)
			}
			_, before := t1Expiry(t, ctx, s.DB, sum[:])
			switch kind {
			case "revoke":
				var id string
				if e = s.DB.QueryRow(ctx, `SELECT id::text FROM administrator_sessions WHERE token_digest=$1`, sum[:]).Scan(&id); e != nil {
					t.Fatal(e)
				}
				if _, e = repo.RevokeSession(ctx, id, sum[:]); e != nil {
					t.Fatal(e)
				}
			case "password":
				if e = repo.ChangePassword(ctx, a, "synthetic-password", "changed-password"); e != nil {
					t.Fatal(e)
				}
			case "administrator reset":
				if _, e = s.DB.Exec(ctx, `UPDATE administrator_accounts SET credential_version=credential_version+1,must_change_password=false,temporary_password_expires_at=NULL WHERE id=$1`, a.ID); e != nil {
					t.Fatal(e)
				}
			case "owner recovery":
				marker := filepath.Join(t.TempDir(), "password")
				if e = os.WriteFile(marker, nil, 0600); e != nil {
					t.Fatal(e)
				}
				if applied, e := ownerrecovery.Apply(ctx, s.DB, marker, io.Discard); e != nil || !applied {
					t.Fatal(applied, e)
				}
			case "temporary expired":
				if _, e = s.DB.Exec(ctx, `UPDATE administrator_accounts SET must_change_password=true,temporary_password_expires_at=now()-interval '1 second' WHERE id=$1`, a.ID); e != nil {
					t.Fatal(e)
				}
			}
			w := t1AdminCall(New(c, Dependencies{DB: s.DB}), token, "/api/v1/auth/session")
			if w.Code != 401 || len(w.Result().Cookies()) != 0 {
				t.Fatal("invalid session authorized or renewed", w.Code)
			}
			_, after := t1Expiry(t, ctx, s.DB, sum[:])
			if !before.Equal(after) {
				t.Fatal("invalidated expiry changed")
			}
		})
	}
}
func TestT1ConcurrentRequestsRefreshOneCookie(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: s.DB}
	c := cfg()
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	sum := sha256.Sum256([]byte(token))
	if _, e := repo.Register(ctx, "t1_parallel", "synthetic-password", token, accounts.SessionMetadata{IP: "8.8.8.8", UserAgent: "T1/1"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `UPDATE administrator_sessions SET created_at=now()-interval '2 hours',expires_at=now()+interval '6 days 22 hours',last_active_at=now() WHERE token_digest=$1`, sum[:]); e != nil {
		t.Fatal(e)
	}
	h := New(c, Dependencies{DB: s.DB})
	ch := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); ch <- t1AdminCall(h, token, "/api/v1/auth/session") }()
	}
	wg.Wait()
	close(ch)
	cookies := 0
	for w := range ch {
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		cookies += len(w.Result().Cookies())
		var data any
		if json.Unmarshal(w.Body.Bytes(), &data) != nil {
			t.Fatal("invalid response")
		}
	}
	if cookies != 1 {
		t.Fatal("expected single renewal", cookies)
	}
}

func TestT1FailedRenewalDoesNotAdvertiseCookieSuccess(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: s.DB}
	c := cfg()
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	sum := sha256.Sum256([]byte(token))
	if _, e := repo.Register(ctx, "t1_failed_touch", "synthetic-password", token); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `UPDATE administrator_sessions SET expires_at=now()+interval '1 hour' WHERE token_digest=$1`, sum[:]); e != nil {
		t.Fatal(e)
	}
	_, before := t1Expiry(t, ctx, s.DB, sum[:])
	if _, e := s.DB.Exec(ctx, `CREATE FUNCTION reject_t1_renewal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic renewal unavailable'; END $$;CREATE TRIGGER reject_t1_renewal BEFORE UPDATE ON administrator_sessions FOR EACH ROW EXECUTE FUNCTION reject_t1_renewal()`); e != nil {
		t.Fatal(e)
	}
	w := t1AdminCall(New(c, Dependencies{DB: s.DB}), token, "/api/v1/auth/session")
	if w.Code != 200 || len(w.Result().Cookies()) != 0 {
		t.Fatal("valid auth must remain available without a fake cookie renewal", w.Code)
	}
	_, after := t1Expiry(t, ctx, s.DB, sum[:])
	if !before.Equal(after) {
		t.Fatal("failed SQL changed expiry")
	}
}
