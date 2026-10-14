package mediausers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"
)

const CookieName = "media_tool_member_session"
const Lifetime = 8 * time.Hour

func NewToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func Token(r *http.Request) string {
	cookie, err := r.Cookie(CookieName)
	if err != nil || len(cookie.Value) != 43 {
		return ""
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		return ""
	}
	return cookie.Value
}
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/api/v1/member", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(Lifetime.Seconds())})
}
func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/api/v1/member", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
