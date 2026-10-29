package networkproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestProxyScopeAndNoSilentFallback(t *testing.T) {
	hits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Host != "target.invalid" {
			t.Error(r.URL.Host)
		}
		io.WriteString(w, "proxied")
	}))
	defer proxy.Close()
	r := recordAt(proxy.URL)
	r.TMDB = true
	client, err := Client(r, "tmdb")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("http://target.invalid/owned")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if hits != 1 {
		t.Fatal(hits)
	}
	direct, _ := Client(r, "ptsite")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "direct") }))
	defer target.Close()
	response, err = direct.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if hits != 1 {
		t.Fatal("scope leaked")
	}
	badRecord := recordAt("http://127.0.0.1:1")
	badRecord.TMDB = true
	bad, _ := Client(badRecord, "tmdb")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", target.URL, nil)
	if _, err = bad.Do(req); err == nil {
		t.Fatal("fell back to direct")
	}
	if strings.Contains(ErrInvalid.Error(), "pass") {
		t.Fatal("secret leakage")
	}
}

// Test listeners return URLs; production accepts only structured fields.
func recordAt(raw string) Record {
	u, e := url.Parse(raw)
	if e != nil {
		panic(e)
	}
	port, _ := strconv.Atoi(u.Port())
	r := Record{Fields: Fields{Protocol: u.Scheme, Host: u.Hostname(), Port: port}, Configured: true}
	if u.User != nil {
		r.Username = u.User.Username()
		r.Password, _ = u.User.Password()
		r.HasPassword = r.Password != ""
	}
	return r
}
