package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type pollTransport func(*http.Request) (*http.Response, error)

func (f pollTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestInboundBoundsRedactsErrorsAndDoesNotFollowRedirect(t *testing.T) {
	for _, body := range []string{`{"ok":false,"description":"SECRET"}`, strings.Repeat("x", (1<<20)+1), `{"ok":true,"result":[{"update_id":-1}]}`, `{"ok":true,"result":[{"update_id":1,"message":{"text":"/hello"}}]}`} {
		c := Client{Token: "SECRET", HTTP: &http.Client{Transport: pollTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.telegram.org" {
				t.Fatal("redirected token")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}}
		updates, e := c.Updates(context.Background(), 0)
		if strings.Contains(body, `"text":"/hello"`) {
			if e != nil || len(updates) != 1 {
				t.Fatal(e)
			}
		} else if e == nil || strings.Contains(e.Error(), "SECRET") {
			t.Fatal("unbounded or raw error")
		}
	}
	calls := 0
	c := Client{Token: "SECRET", HTTP: &http.Client{Transport: pollTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://other.invalid/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, e := c.Updates(context.Background(), 0); e == nil || calls != 1 {
		t.Fatal("followed redirect", calls, e)
	}
}
