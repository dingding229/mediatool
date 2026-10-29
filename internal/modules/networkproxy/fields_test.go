package networkproxy

import (
	"encoding/json"
	"errors"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestStructuredFieldsValidationAndEscaping(t *testing.T) {
	good := Record{Fields: Fields{Protocol: "http", Host: "proxy.example", Port: 7890, Username: "user@ /?#%"}, Password: "p@:/?#% 中文"}
	for _, r := range []Record{good, {Fields: Fields{Protocol: "socks5", Host: "2001:db8::1", Port: 1080}}, {}} {
		if e := Validate(r); e != nil {
			t.Fatal(e)
		}
	}
	u, e := url.Parse(good.proxyURL())
	if e != nil || u.User.Username() != good.Username {
		t.Fatal("username escaping")
	}
	p, _ := u.User.Password()
	if p != good.Password {
		t.Fatal("password escaping")
	}
	ipv6 := Record{Fields: Fields{Protocol: "http", Host: "2001:db8::1", Port: 80}}
	if ipv6.proxyURL() != "http://[2001:db8::1]:80" {
		t.Fatal("IPv6 authority")
	}
	for _, patch := range []Fields{{Protocol: "https", Host: "proxy", Port: 443}, {Protocol: "socks5h", Host: "proxy", Port: 1080}, {Protocol: "http", Host: "http://proxy", Port: 80}, {Protocol: "http", Host: "proxy/path", Port: 80}, {Protocol: "http", Host: "proxy?x", Port: 80}, {Protocol: "http", Host: "a b", Port: 80}, {Protocol: "http", Host: "-bad", Port: 80}, {Protocol: "http", Host: "proxy", Port: 0}, {Protocol: "http", Host: "proxy", Port: 65536}} {
		if Validate(Record{Fields: patch}) == nil {
			t.Fatal("invalid field accepted", patch)
		}
	}
	for _, patch := range []Record{{Fields: good.Fields, Password: "bad\nsecret"}, {Fields: Fields{Protocol: "http", Host: "proxy", Port: 80, Username: "u:p"}}, {Fields: Fields{Protocol: "http", Host: "proxy", Port: 80}, Password: "orphan"}, {TMDB: true}} {
		if Validate(patch) == nil {
			t.Fatal("invalid credentials or empty selected route accepted")
		}
	}
}
func TestPasswordPreservationAndIdentityFence(t *testing.T) {
	v, _ := secrets.New(strings.Repeat("9", 64))
	repo := Repository{Vault: v}
	old := Record{Fields: Fields{Protocol: "http", Host: "proxy", Port: 80, Username: "user"}, Password: "owned-secret", HasPassword: true, Configured: true}
	in := Update{Fields: old.Fields, TMDB: true}
	raw, e := repo.prepare(old, nil, in)
	if e != nil {
		t.Fatal(e)
	}
	next := repo.open(Record{}, raw)
	if next.Password != old.Password || !next.HasPassword || next.NeedsInput {
		t.Fatal("password not retained")
	}
	public, _ := json.Marshal(next)
	if strings.Contains(string(public), "owned-secret") || strings.Contains(string(public), `"password"`) || strings.Contains(string(public), `"url"`) {
		t.Fatal("secret exposed")
	}
	for _, field := range []string{"host", "port", "username", "protocol"} {
		changed := in
		switch field {
		case "host":
			changed.Host = "new-proxy"
		case "port":
			changed.Port++
		case "username":
			changed.Username = "other"
		case "protocol":
			changed.Protocol = "socks5"
		}
		if _, e = repo.prepare(old, nil, changed); !errors.Is(e, ErrPassword) {
			t.Fatal("old password forwarded", field, e)
		}
		changed.Password = "replacement"
		if _, e = repo.prepare(old, nil, changed); e != nil {
			t.Fatal(e)
		}
	}
	in.ClearPassword = true
	raw, e = repo.prepare(old, nil, in)
	if e != nil || repo.open(Record{}, raw).HasPassword {
		t.Fatal("clear password failed")
	}
	in.Password = "new"
	if _, e = repo.prepare(old, nil, in); e == nil {
		t.Fatal("conflicting password intent")
	}
	in = Update{Clear: true}
	raw, e = repo.prepare(old, nil, in)
	if e != nil || len(raw) != 0 {
		t.Fatal("clear settings failed")
	}
	in.Sites = true
	if _, e = repo.prepare(old, nil, in); e == nil {
		t.Fatal("empty selected proxy")
	}
	in = Update{Fields: Fields{Protocol: "socks5", Host: "[2001:DB8::1]", Port: 1080}}
	raw, e = repo.prepare(Record{}, nil, in)
	if e != nil || repo.open(Record{}, raw).Host != "2001:db8::1" {
		t.Fatal("IPv6 normalization")
	}
}
func TestUnknownPayloadIsPreservedAndSelectedRoutesFailClosed(t *testing.T) {
	v, _ := secrets.New(strings.Repeat("9", 64))
	repo := Repository{Vault: v}
	for _, values := range []map[string]string{{"url": "https://old:secret@proxy:443"}, {"url": "socks5h://proxy:1080"}, {"v": "3"}, {"v": "2", "url": "http://proxy"}, {"v": "2", "protocol": "https", "host": "proxy", "port": "443"}, {"v": "2", "protocol": "http", "host": "proxy", "port": "bad"}} {
		raw, _ := v.Seal("network-proxy:main", values)
		old := repo.open(Record{TMDB: true, Sites: true}, raw)
		if !old.NeedsInput || !old.Configured || old.Host != "" || old.Password != "" {
			t.Fatal("legacy payload interpreted")
		}
		for _, scope := range []string{"tmdb", "ptsite", "plugin"} {
			if _, e := Client(old, scope); !errors.Is(e, ErrNeedsInput) {
				t.Fatal("not fail closed", scope, e)
			}
		}
		retained, e := repo.prepare(old, raw, Update{TMDB: true, Sites: true})
		if e != nil || string(retained) != string(raw) {
			t.Fatal("cipher changed")
		}
		old.TMDB = false
		client, e := Client(old, "tmdb")
		if e != nil || client.Transport.(*http.Transport).Proxy != nil {
			t.Fatal("unselected scope changed")
		}
		if _, e = repo.prepare(old, raw, Update{Fields: Fields{Protocol: "http", Host: "replacement", Port: 80}}); !errors.Is(e, ErrNeedsInput) {
			t.Fatal("old unknown credential intent")
		}
		if _, e = repo.prepare(old, raw, Update{Fields: Fields{Protocol: "http", Host: "replacement", Port: 80}, ClearPassword: true}); e != nil {
			t.Fatal(e)
		}
	}
	bad := repo.open(Record{TMDB: true}, []byte("unreadable ciphertext"))
	if !bad.NeedsInput {
		t.Fatal("corrupt payload")
	}
}
