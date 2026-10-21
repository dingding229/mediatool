package sites

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
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
	if strings.Contains(out.Body, "private") || out.Headers["Set-Cookie"] != "" {
		t.Fatal("credential leak", out)
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
	if out, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/large", Method: "GET", MaxResponseBytes: 32}); e != nil || len(out.Body) != 32 || !out.Truncated || len(out.Headers) > 1 {
		t.Fatal("bounded truncation failed", out, e)
	}
	if _, e = c.Controlled(ctx, ControlledRequest{Origin: server.URL, Path: "/", Method: "GET", Raw: true}); e == nil {
		t.Fatal("raw credentials possible")
	}
}

func TestControlledBinaryUTF8JSONReceiptAndLowerLimits(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "binary-ascii")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Language", "zh")
		w.Header().Set("Authorization", "must-not-export")
		_, _ = io.WriteString(w, strings.Repeat("中<", 100000))
	}))
	defer fake.Close()
	c := Client{Account: Account{Endpoint: fake.URL}, HTTP: fake.Client()}
	if _, e := c.Controlled(context.Background(), ControlledRequest{Origin: fake.URL, Path: "/binary", Method: "GET"}); e == nil {
		t.Fatal("binary returned")
	}
	for _, limit := range []int{7, 0} {
		v, e := c.Controlled(context.Background(), ControlledRequest{Origin: fake.URL, Path: "/text", Method: "GET", MaxResponseBytes: limit})
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(v)
		if !v.Truncated || !utf8.ValidString(v.Body) || len(raw) > 96<<10 || limit > 0 && len(v.Body) > limit || v.Headers["Content-Language"] != "zh" || v.Headers["Authorization"] != "" {
			t.Fatal("response bounds", len(raw), v.Headers, e)
		}
	}
}
