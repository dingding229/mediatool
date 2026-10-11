package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresAdministratorRegistrationLoginLogoutAndPasswordChange(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	cleanup := `TRUNCATE administrator_accounts,administrator_auth_limits CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	c := cfg()
	logs := &bytes.Buffer{}
	h := New(c, Dependencies{DB: db, Logger: slog.New(slog.NewTextHandler(logs, nil))})
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Requested-With", "media-tool")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/v1/auth/setup", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"registrationOpen":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	mismatch := `{"username":"media_admin","password":"valid-password","confirmPassword":"different-password"}`
	if w := call("POST", "/api/v1/auth/register", mismatch, nil); w.Code != 400 {
		t.Fatal("confirmation mismatch accepted")
	}
	for _, invalid := range []string{strings.Repeat("x", 5), strings.Repeat("x", 21)} {
		body, _ := json.Marshal(map[string]string{"username": "media_admin", "password": invalid, "confirmPassword": invalid})
		if w := call("POST", "/api/v1/auth/register", string(body), nil); w.Code != 400 || !strings.Contains(w.Body.String(), "6–20") {
			t.Fatal("invalid registration length accepted", w.Code)
		}
	}
	if w := call("GET", "/api/v1/auth/setup", "", nil); !strings.Contains(w.Body.String(), `"passwordMinLength":6`) || !strings.Contains(w.Body.String(), `"passwordMaxLength":20`) {
		t.Fatal("setup advertised old policy")
	}
	password := "abc123" // inclusive minimum; new password below tests inclusive maximum.
	body, _ := json.Marshal(map[string]string{"username": "Media_Admin", "password": password, "confirmPassword": password})
	w := call("POST", "/api/v1/auth/register", string(body), nil)
	if w.Code != 201 || strings.Contains(w.Body.String(), password) {
		t.Fatal("registration failed or leaked", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie missing safe flags")
	}
	first := cookies[0]
	var accountID, hash string
	db.QueryRow(ctx, `SELECT id::text,password_hash FROM administrator_accounts`).Scan(&accountID, &hash)
	if strings.Contains(hash, password) || !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatal("plaintext password")
	}
	if w = call("POST", "/api/v1/auth/register", string(body), nil); w.Code != 409 {
		t.Fatal("second admin accepted")
	}
	if w = call("GET", "/api/v1/auth/session", "", first); w.Code != 200 || !strings.Contains(w.Body.String(), "media_admin") {
		t.Fatal("account session unavailable")
	}
	legacy := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	legacy.Header.Set("Authorization", "Bearer "+c.SessionKey)
	legacyReply := httptest.NewRecorder()
	h.ServeHTTP(legacyReply, legacy)
	if legacyReply.Code != 401 {
		t.Fatal("machine key still authenticates")
	}
	invalidBody := `{"username":"media_admin","password":"wrong-password-fixture"}`
	if w = call("POST", "/api/v1/auth/login", invalidBody, nil); w.Code != 401 {
		t.Fatal("wrong password accepted")
	}
	if w = call("POST", "/api/v1/auth/login", `{"username":"nonexistent","password":"wrong-password-fixture"}`, nil); w.Code != 401 || !strings.Contains(w.Body.String(), "账号或密码不正确") {
		t.Fatal("username enumeration response")
	}
	login, _ := json.Marshal(map[string]string{"username": "MEDIA_ADMIN", "password": password})
	w = call("POST", "/api/v1/auth/login", string(login), nil)
	if w.Code != 200 {
		t.Fatal("password login failed", w.Code)
	}
	second := w.Result().Cookies()[0]
	if w = call("POST", "/api/v1/auth/logout", "{}", first); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if w = call("GET", "/api/v1/auth/session", "", first); w.Code != 401 {
		t.Fatal("revoked cookie replay")
	}
	if w = call("GET", "/api/v1/auth/session", "", second); w.Code != 200 {
		t.Fatal("logout revoked unrelated session")
	}
	for _, invalid := range []string{strings.Repeat("x", 5), strings.Repeat("x", 21)} {
		body, _ := json.Marshal(map[string]string{"currentPassword": password, "newPassword": invalid, "confirmPassword": invalid})
		if w := call("POST", "/api/v1/auth/password", string(body), second); w.Code != 400 {
			t.Fatal("invalid changed password length accepted", w.Code)
		}
	}
	next := strings.Repeat("x", 20) // inclusive maximum
	change, _ := json.Marshal(map[string]string{"currentPassword": password, "newPassword": next, "confirmPassword": next})
	if w = call("POST", "/api/v1/auth/password", string(change), second); w.Code != 200 {
		t.Fatal("password change failed", w.Code)
	}
	if w = call("GET", "/api/v1/auth/session", "", second); w.Code != 401 {
		t.Fatal("password change left session active")
	}
	if w = call("POST", "/api/v1/auth/login", string(login), nil); w.Code != 401 {
		t.Fatal("old password accepted")
	}
	fresh, _ := json.Marshal(map[string]string{"username": "media_admin", "password": next})
	if w = call("POST", "/api/v1/auth/login", string(fresh), nil); w.Code != 200 {
		t.Fatal("new password refused")
	}
	// A correctly signed but unrecorded cookie must never authenticate.
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if w = call("GET", "/api/v1/auth/session", "", &http.Cookie{Name: auth.CookieName, Value: token}); w.Code != 401 {
		t.Fatal("signature alone bypassed session DB")
	}
	if strings.Contains(logs.String(), password) || strings.Contains(logs.String(), next) || strings.Contains(logs.String(), first.Value) {
		t.Fatal("password/cookie logged")
	}
}
func TestPostgresConcurrentRegistrationHasOneWinnerAndDurableRateLimit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB required")
	}
	cleanup := `TRUNCATE administrator_accounts,administrator_auth_limits CASCADE`
	db.Exec(ctx, cleanup)
	defer db.Exec(ctx, cleanup)
	r := accounts.Repository{DB: db}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			_, err := r.Register(ctx, "admin_fixture", "Owned-pass-2026", "synthetic-session-"+time.Now().String())
			results <- err
		})
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if err != accounts.ErrClosed {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("registration race", winners)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*)FROM administrator_accounts`).Scan(&count)
	if count != 1 {
		t.Fatal("singleton not enforced")
	}
	for i := 0; i < 21; i++ {
		allowed, err := r.TakeAttempt(ctx, cfg().SessionKey, "ip:owned-fixture", 20)
		if err != nil || allowed != (i < 20) {
			t.Fatal("rate limiter failed", i, err)
		}
	}
	// A new repository/server sees the same throttle, not an empty in-memory map.
	if allowed, err := (accounts.Repository{DB: db}).TakeAttempt(ctx, cfg().SessionKey, "ip:owned-fixture", 20); err != nil || allowed {
		t.Fatal("restart bypassed throttle")
	}
	var digest []byte
	db.QueryRow(ctx, `SELECT scope_digest FROM administrator_auth_limits LIMIT 1`).Scan(&digest)
	if len(digest) != sha256.Size || strings.Contains(string(digest), "owned") {
		t.Fatal("raw limiter scope stored")
	}
}
