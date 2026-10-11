package auth

import (
	"net/http/httptest"
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
