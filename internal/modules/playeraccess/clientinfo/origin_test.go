package clientinfo

import (
	"net/http/httptest"
	"testing"
)

func TestRequestOriginTrustsOnlyImmediateProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "http://direct.invalid/x", nil)
	r.RemoteAddr = "10.0.0.2:123"
	r.Header.Set("X-Forwarded-Host", "public.invalid:25018")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := RequestOrigin(r, nil); got != "http://direct.invalid" {
		t.Fatal(got)
	}
	if got := RequestOrigin(r, []string{"10.0.0.0/24"}); got != "https://public.invalid:25018" {
		t.Fatal(got)
	}
	r.Header.Set("X-Forwarded-Host", "public.invalid,evil.invalid")
	if got := RequestOrigin(r, []string{"10.0.0.0/24"}); got != "http://direct.invalid" {
		t.Fatal(got)
	}
}
