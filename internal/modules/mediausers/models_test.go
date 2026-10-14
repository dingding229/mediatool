package mediausers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidationDefaultsAndPermissions(t *testing.T) {
	for _, password := range []string{"abc123", strings.Repeat("中", 20)} {
		in := Input{Username: "Member.One", Password: password, Enabled: true}
		if err := in.Validate(true); err != nil || in.Username != "member.one" || in.DisplayName != "member.one" {
			t.Fatal(in, err)
		}
	}
	for _, in := range []Input{{Username: "a", Password: "abc123"}, {Username: "member", Password: "abc12"}, {Username: "member", Password: strings.Repeat("x", 21)}, {Username: "member", Password: "abc123", Grants: []Grant{{LibraryID: "emby"}}}, {Username: "member", Password: "abc123", Grants: []Grant{{LibraryID: "movies"}, {LibraryID: "movies"}}}, {Username: "member", Password: "abc123", DisplayName: "bad\nname"}} {
		if in.Validate(true) == nil {
			t.Fatal("accepted invalid input")
		}
	}
	in := Input{Username: "member", Revision: 1}
	if in.Validate(false) != nil {
		t.Fatal("valid edit rejected")
	}
	in.Password = "abc123"
	if in.Validate(false) == nil {
		t.Fatal("edit bypasses reset flow")
	}
}
func TestMemberCookiesAreIndependentAndStrict(t *testing.T) {
	first, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewToken()
	if first == second || len(first) != 43 {
		t.Fatal("token entropy")
	}
	w := httptest.NewRecorder()
	SetCookie(w, first, true)
	c := w.Result().Cookies()[0]
	if c.Name != "media_tool_member_session" || c.Path != "/api/v1/member" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 28800 {
		t.Fatal(c)
	}
	r := httptest.NewRequest("GET", "/api/v1/member/auth/session", nil)
	r.AddCookie(c)
	if Token(r) != first {
		t.Fatal("cookie not recognized")
	}
	for _, value := range []string{"", strings.Repeat("!", 43), strings.Repeat("a", 44)} {
		rr := httptest.NewRequest("GET", "/", nil)
		rr.AddCookie(&http.Cookie{Name: CookieName, Value: value})
		if Token(rr) != "" {
			t.Fatal("invalid token")
		}
	}
	w = httptest.NewRecorder()
	ClearCookie(w, true)
	if w.Result().Cookies()[0].MaxAge != -1 || w.Result().Cookies()[0].Path != c.Path {
		t.Fatal("incorrect clearing path")
	}
}
