package sites

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestControlledRequestExactOriginCredentialsBoundsAndChallenge(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Cookie") != "session=private-cookie" || r.Header.Get("X-Api-Key") != "private-key" {
			t.Error("host auth missing")
		}
		w.Header().Set("Set-Cookie", "secret-response")
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "https://foreign.invalid/", 302)
		case "/large":
			_, _ = io.WriteString(w, strings.Repeat("x", (1<<20)+1))
		case "/challenge":
			_, _ = io.WriteString(w, "captcha")
		default:
			_, _ = io.WriteString(w, "private-key private-cookie done")
		}
	}))
	defer server.Close()
	c := Client{Account: Account{Endpoint: server.URL, Cookie: "session=private-cookie", APIKey: "private-key"}, HTTP: server.Client()}
	ctx := context.Background()
	for _, path := range []string{"https://foreign.invalid/", "//foreign.invalid/", "/\\evil", "/test#fragment"} {
		before := hits.Load()
		if _, e := c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: path, Method: "GET"}); e == nil || before != hits.Load() {
			t.Fatal("invalid path fetched", path, e)
		}
	}
	out, e := c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/checkin", Method: "POST", SuccessMarker: "done"})
	if e != nil || !out.Matched || out.State != "matched" {
		t.Fatal(e, out)
	}
	out, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/", Method: "GET", SuccessMarker: "private-key"})
	if e != nil || out.Matched {
		t.Fatal("credential matching oracle", e, out)
	}
	out, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/challenge", Method: "GET"})
	if e != nil || out.State != "needs_action" {
		t.Fatal(e, out)
	}
	out, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/redirect", Method: "GET"})
	if e != nil || out.Status != 302 || out.Matched {
		t.Fatal(e, out)
	}
	if _, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/large", Method: "GET"}); e == nil {
		t.Fatal("oversize allowed")
	}
	if _, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/", Method: "GET", Raw: true}); e == nil {
		t.Fatal("raw credentials possible")
	}
}
