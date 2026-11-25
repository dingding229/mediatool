package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"media-tool/internal/modules/accounts"
	"media-tool/tests/fixtures"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRP31StartupInitializationExpiryRotationAndRollback(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: store.DB}
	first := fixtures.Initialization(t, ctx, repo)
	var stored []byte
	var remainingSeconds float64
	if err := store.DB.QueryRow(ctx, `SELECT token_digest, extract(epoch FROM expires_at-clock_timestamp())::float8 FROM administrator_initialization`).Scan(&stored, &remainingSeconds); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(first))
	if !bytes.Equal(stored, digest[:]) || remainingSeconds > 86400 || remainingSeconds < 86300 {
		t.Fatal("startup stored unexpected digest or lifetime")
	}
	if _, err := store.DB.Exec(ctx, `UPDATE administrator_initialization SET expires_at=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Register(ctx, "setup_admin", "TestPassword1", "session", first); !errors.Is(err, accounts.ErrInitialization) {
		t.Fatal("expired initialization token accepted", err)
	}
	second := fixtures.Initialization(t, ctx, repo)
	third := fixtures.Initialization(t, ctx, repo)
	if second == third {
		t.Fatal("empty restart did not rotate token")
	}
	if _, err := repo.Register(ctx, "setup_admin", "TestPassword1", "session", second); !errors.Is(err, accounts.ErrInitialization) {
		t.Fatal("rotated initialization token accepted", err)
	}
	if _, err := repo.Register(ctx, "setup_admin", "short", "session", third); err == nil {
		t.Fatal("invalid password unexpectedly registered")
	}
	if _, err := repo.Register(ctx, "setup_admin", "TestPassword1", "session", third); err != nil {
		t.Fatal("failed registration consumed the token", err)
	}
	var console bytes.Buffer
	if err := repo.PrintInitialization(ctx, &console); err != nil || console.Len() != 0 {
		t.Fatal("registered database issued another token", err)
	}
}

func TestRP31AuthenticationBudgets(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: store.DB}
	c := cfg()
	h := New(c, Dependencies{DB: store.DB})
	// Start far enough from the one-minute expiry. Only this fixture uses the schema.
	if _, err := fixtures.Register(t, ctx, repo, "budget_admin", "TestPassword1", "seed-session"); err != nil {
		t.Fatal(err)
	}
	admit := func(source string) bool {
		t.Helper()
		ok, err := repo.AdmitAuthentication(ctx, c.SessionKey, source)
		if err != nil {
			t.Error(err)
		}
		return ok
	}
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for i := 0; i < 25; i++ {
		wg.Go(func() {
			if admit("10.0.0.1") {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 20 {
		t.Fatal("source limit raced", accepted.Load())
	}
	call := func(path, ip, body string) int {
		r := httptest.NewRequest("POST", "/api/v1/auth/"+path, strings.NewReader(body)).WithContext(ctx)
		r.RemoteAddr = ip + ":12345"
		r.Header.Set("X-Requested-With", "media-tool")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	login := `{"username":"budget_admin","password":"TestPassword1"}`
	for i := 0; i < 3; i++ {
		if call("login", "10.0.0.1", login) != 429 || call("login", "10.0.0.2", `{"username":"!","password":"x"}`) != 401 || call("register", "10.0.0.2", login) != 409 {
			t.Fatal("source limit or cheap validation missing")
		}
	}
	globalCount := func() int {
		t.Helper()
		h := hmac.New(sha256.New, []byte(c.SessionKey))
		h.Write([]byte("media-tool:auth-rate:v1:global"))
		var n int
		if err := store.DB.QueryRow(ctx, `SELECT attempts FROM administrator_auth_limits WHERE scope_digest=$1 AND window_start>now()-interval '1 minute'`, h.Sum(nil)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if globalCount() != 20 || call("login", "10.0.0.2", login) != 200 {
		t.Fatal("source denial or cheap request charged the global budget")
	}
	for i := 0; i < 99; i++ {
		if !admit(fmt.Sprintf("10.1.0.%d", i+1)) {
			t.Fatal("lost global capacity", i)
		}
	}
	if admit("10.2.0.1") || globalCount() != 120 {
		t.Fatal("global cap not enforced")
	}
	if _, err := store.DB.Exec(ctx, `UPDATE administrator_auth_limits SET window_start=$1`, time.Now().Add(-2*time.Minute)); err != nil || !admit("10.0.0.1") || globalCount() != 1 {
		t.Fatal("auth limit window failed to reset", err)
	}
}
