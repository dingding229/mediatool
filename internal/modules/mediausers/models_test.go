package mediausers

import (
	"encoding/json"
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

func TestNativePermissionInputValidation(t *testing.T) {
	for _, raw := range []string{
		`{"username":"member","revision":1,"maxStreams":0}`,
		`{"username":"member","revision":1,"maxStreams":21}`,
		`{"username":"member","revision":1,"maxParentalRating":-1}`,
		`{"username":"member","revision":1,"maxParentalRating":101}`,
		`{"username":"member","revision":1,"maxParentalRating":null}`,
		`{"username":"member","revision":1,"MaxParentalRating":65}`,
		`{"username":"member","revision":1,"BlockUnratedItems":[]}`,
		`{"username":"member","revision":1,"grants":[{"libraryId":"g:test","canDownload":true}]}`,
		`{"username":"member","revision":1,"expiresAt":"invalid-date"}`,
		`{"username":"member","revision":1,"unknown":true}`,
	} {
		var in Input
		if err := json.Unmarshal([]byte(raw), &in); err == nil && in.Validate(false) == nil {
			t.Fatal("invalid permission input accepted", raw)
		}
	}
	var omitted, cleared Input
	if err := json.Unmarshal([]byte(`{"username":"member","revision":1}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"username":"member","revision":1,"expiresAt":null}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if omitted.expirySet || !cleared.expirySet {
		t.Fatal("nullable update semantics lost")
	}
}
