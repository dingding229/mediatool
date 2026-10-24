package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "media_tool_session"

// Lifetime is the initial/idle lifetime. PostgreSQL owns sliding expiration.
const Lifetime = 7 * 24 * time.Hour
const AbsoluteLifetime = 30 * 24 * time.Hour

type Auth struct {
	Key, Origin    string
	AllowedOrigins []string
	Secure         bool
}

func Match(secret, supplied string) bool {
	a := sha256.Sum256([]byte(secret))
	b := sha256.Sum256([]byte(supplied))
	return secret != "" && subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
func (a Auth) Authorized(r *http.Request) bool {
	cookie, err := r.Cookie(CookieName)
	return err == nil && a.Verify(cookie.Value, time.Now())
}
func (a Auth) SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && origin != a.Origin {
		allowed := false
		for _, candidate := range a.AllowedOrigins {
			if origin == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return r.Header.Get("X-Requested-With") == "media-tool"
}
func (a Auth) Issue(now time.Time) (string, error) {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	data := strconv.FormatInt(now.Add(Lifetime).Unix(), 10) + "." + hex.EncodeToString(nonce)
	return data + "." + a.signature(data), nil
}

// VerifySignature validates the existing v2 token format and HMAC, including
// legacy tokens carrying an 8h timestamp. It grants no authority on its own:
// database-backed callers MUST check the session row, revocation and version.
func (a Auth) VerifySignature(token string) bool {
	p := strings.Split(token, ".")
	if len(p) != 3 || len(p[1]) != 48 {
		return false
	}
	timestamp, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || timestamp <= 0 {
		return false
	}
	nonce, e := hex.DecodeString(p[1])
	if e != nil || len(nonce) != 24 {
		return false
	}
	return Match(a.signature(p[0]+"."+p[1]), p[2])
}

// Verify retains the nominal timestamp check for standalone/non-PG callers.
// Administrator HTTP authentication uses VerifySignature plus live PG checks.
func (a Auth) Verify(token string, now time.Time) bool {
	if !a.VerifySignature(token) {
		return false
	}
	p := strings.Split(token, ".")
	expiry, _ := strconv.ParseInt(p[0], 10, 64)
	return expiry > now.Unix() && expiry <= now.Add(Lifetime+time.Minute).Unix()
}
func (a Auth) signature(data string) string {
	m := hmac.New(sha256.New, []byte(a.Key))
	m.Write([]byte("media-tool:administrator-session:v2:" + data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (a Auth) SetCookie(w http.ResponseWriter, token string) {
	now := time.Now()
	a.SetCookieUntil(w, token, now.Add(Lifetime), now)
}

// SetCookieUntil uses both the expiry and clock returned by PostgreSQL, so
// initial MaxAge is exactly 7d even if an app/test host clock is slightly skewed.
func (a Auth) SetCookieUntil(w http.ResponseWriter, token string, expiresAt, asOf time.Time) {
	a.setCookieUntil(w, token, expiresAt, asOf)
}
func (a Auth) setCookieUntil(w http.ResponseWriter, token string, expiresAt, now time.Time) {
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		a.ClearCookie(w)
		return
	}
	setSessionCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, Expires: expiresAt.UTC(), MaxAge: int(math.Ceil(remaining.Seconds()))})
}
func (a Auth) ClearCookie(w http.ResponseWriter) {
	setSessionCookie(w, &http.Cookie{Name: CookieName, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

// A later logout/revoke must replace a renewal on the same response, while
// preserving cookies owned by other handlers.
func setSessionCookie(w http.ResponseWriter, cookie *http.Cookie) {
	previous := w.Header().Values("Set-Cookie")
	w.Header().Del("Set-Cookie")
	for _, value := range previous {
		if !strings.HasPrefix(value, CookieName+"=") {
			w.Header().Add("Set-Cookie", value)
		}
	}
	http.SetCookie(w, cookie)
}
