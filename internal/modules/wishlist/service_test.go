package wishlist

import (
	"context"
	"io"
	"media-tool/internal/modules/integrations"
	"net/http"
	"strings"
	"testing"
)

const publicWish = `<div class="grid-view"><div class="item"><div class="info"><ul><li class="title"><a href="https://movie.douban.com/subject/123/">Synthetic / Original</a></li><li class="intro">2026 / CN</li></ul></div></div></div>`

func TestPublicDOMOnlyNoLoginOrUnknownMatch(t *testing.T) {
	for _, tc := range []struct{ body, state string }{{publicWish, "ok"}, {`<form action="login"><input type="password"></form>`, "uncertain"}, {`<input name="captcha-solution">`, "uncertain"}, {`<html>changed unknown markup</html>`, "uncertain"}, {strings.ReplaceAll(publicWish, "movie.douban.com", "private.invalid"), "uncertain"}, {strings.ReplaceAll(publicWish, "https://movie.douban.com", "https://user:secret@movie.douban.com"), "uncertain"}} {
		p, e := Parse([]byte(tc.body), Request{Source: "douban-wish", ID: "public_user", Page: 1})
		if e != nil || p.State != tc.state {
			t.Fatal(tc, p, e)
		}
		if p.State == "ok" && (len(p.Items) != 1 || p.Items[0].Title != "Synthetic" || p.Items[0].Year != "2026" || p.Items[0].TMDBID != 0) {
			t.Fatal(p)
		}
	}
	if _, e := Parse([]byte(strings.Repeat("x", (1<<20)+1)), Request{}); e == nil {
		t.Fatal("unbounded body")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFetchFixedOriginNoCookiesRedirectOrCredentials(t *testing.T) {
	for _, code := range []int{200, 302, 429} {
		calls := 0
		m := &integrations.Manager{OutboundHTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Scheme != "https" || r.URL.Host != "movie.douban.com" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				t.Error("credentials or arbitrary target")
			}
			return &http.Response{StatusCode: code, Header: http.Header{"Set-Cookie": {"PRIVATE_COOKIE"}, "Location": {"https://private.invalid"}}, Body: io.NopCloser(strings.NewReader(publicWish))}, nil
		})}}
		s := Service{Connections: m}
		p, e := s.Fetch(context.Background(), Request{Source: "douban-wish", ID: "public_user", Page: 1})
		if e != nil || calls != 1 {
			t.Fatal(e, calls)
		}
		if code == 200 && p.State != "ok" || code == 302 && p.State != "unavailable" || code == 429 && p.State != "limited" {
			t.Fatal(code, p)
		}
		for _, id := range []string{"../secret", "https://private.invalid", "user?token=secret"} {
			if _, e = s.Fetch(context.Background(), Request{Source: "douban-wish", ID: id, Page: 1}); e == nil {
				t.Fatal(id)
			}
		}
	}
}
