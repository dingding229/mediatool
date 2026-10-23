package embyapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRequestContextCarriesTrustedClientAddressAndUnverifiedPlaceholders(t *testing.T) {
	r := httptest.NewRequest("GET", "/emby/Users/1?api_key=token", nil)
	r.RemoteAddr = "93.184.216.34:443"
	r.Header.Set("User-Agent", "Infuse/1")
	info, err := NewRequestContext(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.UserAgent != "Infuse/1" || info.Address.ClientIP != "93.184.216.34" || info.Address.Source != "direct" || !info.Address.Public || info.User != nil || info.Device != nil || info.Token != nil {
		t.Fatalf("context=%+v", info)
	}
	with := WithRequestInfo(r, info)
	got, ok := RequestInfo(with.Context())
	if !ok || got.Auth.Token != "token" {
		t.Fatalf("request context=%+v ok=%v", got, ok)
	}
}
func TestRequestContextRejectsInvalidTrustedProxyConfigurationAndDuplicateUA(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("User-Agent", "one")
	r.Header.Add("User-Agent", "two")
	if _, err := NewRequestContext(r, nil); err == nil {
		t.Fatal("duplicate User-Agent accepted")
	}
	r = httptest.NewRequest("GET", "/", nil)
	if _, err := NewRequestContext(r, []string{"10.0.0.1/24"}); err == nil {
		t.Fatal("unmasked proxy accepted")
	}
}

func TestRequestContextUsesExistingProxyTrustRules(t *testing.T) {
	r := httptest.NewRequest("GET", "/?UserId=forged-user", nil)
	r.RemoteAddr = "10.0.0.2:32100"
	r.Header.Set("User-Agent", "Actual-Player-UA")
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	r.Header.Set("X-Real-IP", "1.1.1.1")
	info, err := NewRequestContext(r, nil)
	if err != nil || info.Address.ClientIP != "" || info.Address.Source != "direct" || info.Address.Public {
		t.Fatal("untrusted forwarded IP accepted", info.Address, err)
	}
	info, err = NewRequestContext(r, []string{"10.0.0.0/24"})
	if err != nil || info.Address.ClientIP != "8.8.8.8" || info.Address.ConnectionIP != "" || info.Address.Source != "trusted_proxy" || !info.Address.Public || info.UserAgent != "Actual-Player-UA" {
		t.Fatal(info.Address, err)
	}
	if info.Auth.UserID != "forged-user" || info.User != nil || info.Device != nil || info.Token != nil {
		t.Fatal("unverified input became identity")
	}
	data, err := json.Marshal(info)
	if err != nil || string(data) != "{}" {
		t.Fatal("internal context leaked through JSON", string(data), err)
	}
	r.Header.Set("X-Forwarded-For", "invalid,8.8.8.8")
	info, err = NewRequestContext(r, []string{"10.0.0.0/24"})
	if err != nil || info.Address.Source != "invalid_forwarded_chain" || info.Address.Public {
		t.Fatal("malformed forwarded chain", info.Address, err)
	}
}
