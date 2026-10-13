package networkproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	r := Record{URL: proxy.URL, TMDB: true}
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
	for _, value := range []string{"file:///tmp/proxy", "http://proxy/path", "http://proxy?q=1", "http://proxy#frag", "http://proxy:0", "socks4://proxy:1080", "\nhttp://proxy"} {
		if Validate(Record{URL: value}) == nil {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"http://user:pass@proxy:7890", "https://proxy:8443", "socks5://proxy:1080", "socks5h://proxy:1080"} {
		if Validate(Record{URL: value}) != nil {
			t.Fatal(value)
		}
	}
	bad, _ := Client(Record{URL: "http://127.0.0.1:1", TMDB: true}, "tmdb")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", target.URL, nil)
	if _, err = bad.Do(req); err == nil {
		t.Fatal("fell back to direct")
	}
	if strings.Contains(ErrInvalid.Error(), "pass") {
		t.Fatal("secret leakage")
	}
}
