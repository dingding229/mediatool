package integrations

import (
	"errors"
	"testing"
)

func TestPTEncryptedRequestHeadersAreAnExplicitSecretNotAPublicSetting(t *testing.T) {
	in := Input{Kind: "ptsite", Name: "Owned", Settings: map[string]any{"endpoint": "https://owned.invalid", "definitionId": "owned"}, Secrets: map[string]string{"requestHeaders": `{"Authorization":"Bearer PRIVATE-AUTH","X-Csrf-Token":"PRIVATE-CSRF"}`}}
	if err := Validate(in); err != nil {
		t.Fatal("explicit encrypted PT headers rejected", err)
	}
	in.Secrets = nil
	in.Settings["requestHeaders"] = `{"Authorization":"PRIVATE-AUTH"}`
	if err := Validate(in); err == nil {
		t.Fatal("PT private headers accepted in public settings")
	}
}

func TestPTHeaderSecretStrictValidationNeverEchoesValues(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{"Authorization":42}`, `{"Authorization":null}`,
		`{"Authorization":"PRIVATE-ONE","Authorization":"PRIVATE-TWO"}`,
		`{"Authorization":"PRIVATE-ONE","authorization":"PRIVATE-TWO"}`,
		`{"Authorization":""}`, `{"Host":"PRIVATE-HOST"}`, `{"Cookie":"PRIVATE-COOKIE"}`,
		`{"Content-Length":"100"}`, `{"Connection":"PRIVATE-CONNECTION"}`,
		`{"Proxy-Authorization":"PRIVATE-PROXY"}`, `{"X-Forwarded-For":"PRIVATE-IP"}`,
		`{"Authorization":"PRIVATE-AUTH\r\nInjected: value"}`,
		`{"Authorization":"PRIVATE-AUTH"} {}`,
	} {
		in := Input{Kind: "ptsite", Name: "Owned", Secrets: map[string]string{"requestHeaders": raw}}
		err := Validate(in)
		if !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid request-header JSON accepted or unsafe error returned", err)
		}
	}
	if err := Validate(Input{Kind: "emby", Name: "Owned", Secrets: map[string]string{"requestHeaders": `{"Authorization":"PRIVATE-AUTH"}`}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("PT header secret accepted for another provider", err)
	}
}
