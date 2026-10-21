package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminSessionEnumerationAndSingleRevoke(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := accounts.Repository{DB: s.DB}
	c := cfg()
	// Real auth repository, not a SessionReader stub; every request must consult PG.
	accountToken, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	account, err := repo.Register(ctx, "b2_admin", "owned-password", accountToken, accounts.SessionMetadata{IP: "192.0.2.5", UserAgent: "Original <UA> 原值"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if err = repo.CreateSession(ctx, account, second, accounts.SessionMetadata{IP: "192.0.2.6", UserAgent: "Second/1.0"}); err != nil {
		t.Fatal(err)
	}
	h := New(c, Dependencies{DB: s.DB})
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.5:1234"
		r.Header.Set("User-Agent", "Original <UA> 原值")
		r.Header.Set("X-Requested-With", "media-tool")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/v1/auth/sessions", "", ""); w.Code != 401 {
		t.Fatal("anonymous enumeration", w.Code)
	}
	w := call("GET", "/api/v1/auth/sessions?page=1", "", accountToken)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Data accounts.SessionPage `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Data.Items) != 2 {
		t.Fatal(err, w.Body.String())
	}
	current, other := "", ""
	for _, row := range out.Data.Items {
		if row.Current {
			current = row.ID
			if row.UserAgent != "Original <UA> 原值" || row.IP != "192.0.2.5" {
				t.Fatal("raw values changed", row)
			}
		} else {
			other = row.ID
		}
	}
	for _, secret := range []string{accountToken, second, "token_digest", "credential_version", "password_hash"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret leaked", secret)
		}
	}
	path := "/api/v1/auth/sessions/" + other + "/revoke"
	if w = call("POST", path, `{"confirm":false}`, accountToken); w.Code != 400 {
		t.Fatal("missing confirmation")
	}
	for range 2 {
		if w = call("POST", path, `{"confirm":true}`, accountToken); w.Code != 200 {
			t.Fatal("idempotent revoke", w.Code, w.Body.String())
		}
	}
	if w = call("GET", "/api/v1/auth/session", "", second); w.Code != 401 {
		t.Fatal("revoked cookie still accepted", w.Code)
	}
	if w = call("GET", "/api/v1/auth/session", "", accountToken); w.Code != 200 {
		t.Fatal("unrelated session revoked")
	}
	w = call("POST", "/api/v1/auth/sessions/"+current+"/revoke", `{"confirm":true}`, accountToken)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"loginRequired":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/v1/auth/session", "", accountToken); w.Code != 401 {
		t.Fatal("current revoke not immediate")
	}
}
