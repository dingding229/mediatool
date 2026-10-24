package auth

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSessionSignatureExpiryAndRotation(t *testing.T) {
	a := Auth{Key: strings.Repeat("a", 64)}
	now := time.Now()
	token, err := a.Issue(now)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Verify(token, now) {
		t.Fatal("valid token rejected")
	}
	if a.Verify(token+"x", now) || a.Verify(token, now.Add(Lifetime+time.Second)) {
		t.Fatal("tamper or expiry accepted")
	}
	if (Auth{Key: strings.Repeat("b", 64)}).Verify(token, now) {
		t.Fatal("rotation did not invalidate")
	}
}
func TestMatch(t *testing.T) {
	if Match("", "") || Match("a", "b") || !Match("a", "a") {
		t.Fatal("comparison failed")
	}
}
func TestSameOrigin(t *testing.T) {
	a := Auth{Origin: "https://media.example.com"}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Requested-With", "media-tool")
	r.Header.Set("Origin", "https://evil.example.com")
	if a.SameOrigin(r) {
		t.Fatal("cross-origin accepted")
	}
	r.Header.Set("Origin", a.Origin)
	if !a.SameOrigin(r) {
		t.Fatal("same-origin rejected")
	}
}

func TestExplicitProxyOrigins(t *testing.T) {
	a := Auth{Origin: "http://127.0.0.1:8080", AllowedOrigins: []string{"https://media.example.com"}}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Requested-With", "media-tool")
	r.Header.Set("Origin", "https://media.example.com")
	if !a.SameOrigin(r) {
		t.Fatal("configured proxy origin rejected")
	}
	r.Header.Del("X-Requested-With")
	if a.SameOrigin(r) {
		t.Fatal("missing CSRF header accepted")
	}
	r.Header.Set("X-Requested-With", "media-tool")
	r.Header.Set("Origin", "https://evil.example.com")
	r.Header.Set("X-Forwarded-Host", "media.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	if a.SameOrigin(r) {
		t.Fatal("forwarded headers bypassed origin check")
	}
}

func TestDatabaseSessionSignatureKeepsLegacyEightHourTokens(t *testing.T) {
	a := Auth{Key: strings.Repeat("a", 64)}
	now := time.Now()
	// A prior version signed this nominal 8h expiry; the PG row may have renewed.
	data := strconv.FormatInt(now.Add(-time.Hour).Unix(), 10) + "." + strings.Repeat("a", 48)
	token := data + "." + a.signature(data)
	if !a.VerifySignature(token) || a.Verify(token, now) {
		t.Fatal("legacy signature and nominal expiration were not separated")
	}
	for _, invalid := range []string{token + "x", data + ".bad", "bad", "0." + strings.Repeat("a", 48) + ".bad", strings.Replace(token, strings.Repeat("a", 48), strings.Repeat("z", 48), 1)} {
		if a.VerifySignature(invalid) {
			t.Fatal("invalid signature or format accepted")
		}
	}
	if (Auth{Key: strings.Repeat("b", 64)}).VerifySignature(token) {
		t.Fatal("key rotation bypassed")
	}
}
func TestCookieUsesAuthoritativeExpiryAndRemainingMaxAge(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	a := Auth{Secure: true}
	for _, remaining := range []time.Duration{Lifetime, 23*time.Hour + 30*time.Minute, 500 * time.Millisecond} {
		w := httptest.NewRecorder()
		expires := now.Add(remaining)
		a.setCookieUntil(w, "synthetic", expires, now)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("cookie count", len(cookies))
		}
		c := cookies[0]
		if c.MaxAge != int(math.Ceil(remaining.Seconds())) || !c.Expires.Equal(expires.Truncate(time.Second)) || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
			t.Fatal("cookie policy/expiry mismatch")
		}
	}
	w := httptest.NewRecorder()
	a.setCookieUntil(w, "synthetic", now, now)
	if w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("expired cookie retained")
	}
	if Lifetime != 7*24*time.Hour || AbsoluteLifetime != 30*24*time.Hour {
		t.Fatal("session lifetime policy")
	}
}

func TestClearCookieReplacesRenewalButPreservesUnrelatedCookies(t *testing.T) {
	a := Auth{}
	w := httptest.NewRecorder()
	http.SetCookie(w, &http.Cookie{Name: "other", Value: "owned"})
	a.SetCookie(w, "synthetic")
	a.ClearCookie(w)
	cookies := w.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "other" || cookies[1].Name != CookieName || cookies[1].MaxAge != -1 || cookies[1].Value != "" {
		t.Fatal("logout did not replace renewal")
	}
}
