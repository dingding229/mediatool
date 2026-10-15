package settings

import "testing"

func TestAccessDomainsRequireCanonicalOrigins(t *testing.T) {
	for _, raw := range []string{"https://admin.example.com", "http://media.example.com:8092", ""} {
		if err := ValidateAccess(Access{ManagementURL: raw}); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"https://user:pass@a.example.com", "https://example.com/watch", "https://example.com/", "https://example.com?a=1", "https://example.com#x", "https://127.0.0.1", "http://localhost:8092", "ftp://example.com", "https://EXAMPLE.com", "https://bad..com", "https://example.com:99999"} {
		if ValidateAccess(Access{PlaybackURL: raw}) == nil {
			t.Fatal("accepted", raw)
		}
	}
}

func TestSeparateMediaOrigins(t *testing.T) {
	if ValidateAccess(Access{ManagementURL: "https://same.example.com", MemberURL: "https://same.example.com"}) == nil {
		t.Fatal("same-root listeners accepted")
	}
	if ValidateAccess(Access{ManagementURL: "https://same.example.com", PlaybackURL: "https://same.example.com"}) != nil {
		t.Fatal("STRM should share management listener when desired")
	}
}

func TestBrowserDefaultPortOrigin(t *testing.T) {
	if OriginKey("https://media.example.com:443") != "https://media.example.com" {
		t.Fatal("browser origin mismatch")
	}
	if ValidateAccess(Access{ManagementURL: "https://same.example.com", MemberURL: "https://same.example.com:443"}) == nil {
		t.Fatal("equivalent origins accepted for separate roots")
	}
}
