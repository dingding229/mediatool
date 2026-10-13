package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "media_tool_session"
const Lifetime = 8 * time.Hour

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
func (a Auth) Verify(token string, now time.Time) bool {
	p := strings.Split(token, ".")
	if len(p) != 3 || len(p[1]) != 48 {
		return false
	}
	expiry, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil || expiry <= now.Unix() || expiry > now.Add(Lifetime+time.Minute).Unix() {
		return false
	}
	return Match(a.signature(p[0]+"."+p[1]), p[2])
}
func (a Auth) signature(data string) string {
	m := hmac.New(sha256.New, []byte(a.Key))
	m.Write([]byte("media-tool:administrator-session:v2:" + data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (a Auth) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: int(Lifetime.Seconds())})
}
func (a Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
