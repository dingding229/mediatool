package embyapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func values(t *testing.T, raw string) Values {
	t.Helper()
	v, err := ParseValues(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestAuthParsesInfuseSenPlayerAndEmbyWebFormats(t *testing.T) {
	cases := []struct {
		name, header                             string
		query                                    string
		client, device, id, version, user, token string
	}{
		{"Infuse", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="device%2D1", Version="7.7.0", Token="token-a", UserId="u1"`, "", "Infuse", "iPhone", "device-1", "7.7.0", "u1", "token-a"},
		{"SenPlayer", `Emby Client=SenPlayer,Device=Android,DeviceId=sen-1,Version=6.2.1,Token=token-b`, "", "SenPlayer", "Android", "sen-1", "6.2.1", "", "token-b"},
		{"Emby Web", `MediaBrowser Client="Emby Web", Device="Chrome", DeviceId="web-1", Version="4.8.10.0", Token="token-c"`, "ApiKey=token-c&UserID=user-7", "Emby Web", "Chrome", "web-1", "4.8.10.0", "user-7", "token-c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/Users/1?"+tc.query, nil)
			r.Header.Set("Authorization", tc.header)
			auth, err := ParseAuth(r, values(t, r.URL.RawQuery))
			if err != nil {
				t.Fatal(err)
			}
			if auth.Client != tc.client || auth.Device != tc.device || auth.DeviceID != tc.id || auth.Version != tc.version || auth.UserID != tc.user || auth.Token != tc.token {
				t.Fatalf("got %+v", auth)
			}
		})
	}
}
func TestAuthSourcesConflictAndLengthOrControlCharactersAreInvalid(t *testing.T) {
	r := httptest.NewRequest("GET", "/Users/1?api_key=two", nil)
	r.Header.Set("X-Emby-Token", "one")
	if _, err := ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("conflicting token accepted")
	}
	r = httptest.NewRequest("GET", "/Users/1", nil)
	r.Header.Set("X-Emby-Authorization", `Client="one",Client="two"`)
	if _, err := ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("conflicting repeated metadata accepted")
	}
	r = httptest.NewRequest("GET", "/Users/1", nil)
	r.Header.Set("X-Emby-Authorization", `Client="`+strings.Repeat("x", 129)+`"`)
	if _, err := ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("overlong client accepted")
	}
	r = httptest.NewRequest("GET", "/Users/1", nil)
	r.Header.Set("X-Emby-Authorization", `Client="ok`+string(rune(1))+`"`)
	if _, err := ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("control character accepted")
	}
}
func TestAuthNoSchemeAuthorizationIsRejectedAndQuotedTokenAccepted(t *testing.T) {
	r := httptest.NewRequest("GET", "/Users/1", nil)
	r.Header.Set("Authorization", `Bearer token`)
	if _, err := ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("Bearer accepted unexpectedly")
	}
	r = httptest.NewRequest("GET", "/Users/1", nil)
	r.Header.Set("X-Emby-Token", `"token"`)
	auth, err := ParseAuth(r, values(t, r.URL.RawQuery))
	if err != nil || auth.Token != "token" {
		t.Fatalf("quoted token: %+v %v", auth, err)
	}
}

func TestAuthEveryTokenSourceAndConflicts(t *testing.T) {
	sources := []struct{ header, value, query string }{
		{"X-Emby-Authorization", `MediaBrowser Token="synthetic-key"`, ""},
		{"Authorization", `Emby Token="synthetic-key"`, ""},
		{"X-Emby-Token", "synthetic-key", ""},
		{"X-MediaBrowser-Token", "synthetic-key", ""},
		{"", "", "api_key=synthetic-key"}, {"", "", "ApiKey=synthetic-key"},
		{"", "", "aPi_KeY=synthetic-key"}, {"", "", "X-Emby-Token=synthetic-key"},
	}
	for _, tc := range sources {
		t.Run(tc.header+tc.query, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/Users/1?"+tc.query, nil)
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			auth, err := ParseAuth(r, values(t, r.URL.RawQuery))
			if err != nil || auth.Token != "synthetic-key" {
				t.Fatalf("source failed %v", err)
			}
			r.Header.Set("X-MediaBrowser-Token", "different-key")
			if tc.header == "X-MediaBrowser-Token" {
				r.URL.RawQuery = "api_key=synthetic-key"
			}
			if _, err = ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
				t.Fatal("conflicting sources accepted")
			}
		})
	}
}
func TestAuthMetadataPriorityAndDelimiterEscapes(t *testing.T) {
	r := httptest.NewRequest("GET", "/?UserId=query-user&ApiKey=token%2520", nil)
	r.Header.Set("X-Emby-Authorization", `Client="Primary", Device="TV, Living \"Room\"+", DeviceId="%E8%AE%BE%E5%A4%87", Token="token%2520"`)
	r.Header.Set("Authorization", `MediaBrowser Client="Secondary",Device="Fallback",Version="1.2",UserId="header-user",Token="token%2520"`)
	a, err := ParseAuth(r, values(t, r.URL.RawQuery))
	if err != nil || a.Client != "Primary" || a.Device != `TV, Living "Room"+` || a.DeviceID != "设备" || a.Version != "1.2" || a.UserID != "header-user" || a.Token != "token%20" {
		t.Fatalf("priority or escaping failed: %+v %v", a, err)
	}
	// The query spelling with and without underscore is one scalar namespace.
	r = httptest.NewRequest("GET", "/?api_key=one&ApiKey=two", nil)
	if _, err = ParseAuth(r, values(t, r.URL.RawQuery)); err == nil {
		t.Fatal("conflicting aliases accepted")
	}
}
func TestAuthLimitsFollowT03CharacterLengths(t *testing.T) {
	for _, tc := range []struct {
		field string
		max   int
	}{{"Client", 128}, {"Device", 256}, {"DeviceId", 256}, {"Version", 64}, {"UserId", 64}, {"Token", 512}} {
		for _, over := range []bool{false, true} {
			t.Run(tc.field+map[bool]string{false: "_boundary", true: "_over"}[over], func(t *testing.T) {
				count := tc.max
				if over {
					count++
				}
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("X-Emby-Authorization", tc.field+`="`+strings.Repeat("中", count)+`"`)
				_, err := ParseAuth(r, values(t, ""))
				if (err != nil) != over {
					t.Fatalf("%s count=%d error=%v", tc.field, count, err)
				}
			})
		}
	}
}
func TestAuthRejectsMalformedInputAndRepeatedHeaderSmuggling(t *testing.T) {
	for _, raw := range []string{`Client="unfinished`, `Client="ok"suffix`, `=missing`, `Token=one,Token=two`, `Client="one",client="two"`, `Device=unquoted"value`, `Device="bad%ZZ"`, `Device="bad%00"`, `Device="bad%C2%85"`, "Client=bad\rname", string([]byte{0xff})} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Emby-Authorization", raw)
		if _, err := ParseAuth(r, values(t, "")); err == nil {
			t.Fatalf("malformed header accepted: %q", raw)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("X-Emby-Token", "one")
	r.Header.Add("X-Emby-Token", "two")
	if _, err := ParseAuth(r, values(t, "")); err == nil {
		t.Fatal("duplicate token headers accepted")
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Emby-Authorization", strings.Repeat("x", 8193))
	if _, err := ParseAuth(r, values(t, "")); err == nil {
		t.Fatal("unbounded header accepted")
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "administrator-session", Value: "not-a-media-token"})
	a, err := ParseAuth(r, values(t, ""))
	if err != nil || a.Token != "" {
		t.Fatal("administrator cookie became media authentication")
	}
}
