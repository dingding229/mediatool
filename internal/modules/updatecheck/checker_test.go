package updatecheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func metadata() Metadata {
	return Metadata{Version: "0.28.0", Commit: strings.Repeat("a", 40), Digest: "sha256:" + strings.Repeat("b", 64), Schema: 79, MinimumSchema: 79, Repository: Repository, Platform: "linux/amd64"}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureClient(t *testing.T, change func(string, map[string]any), status int) *Service {
	t.Helper()
	s := New(nil, nil)
	body, e := ReleaseBody("中文变更：只检查，不更新", metadata())
	if e != nil {
		t.Fatal(e)
	}
	s.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.Scheme != "https" {
			t.Fatal("credentials/write request", r.Method, r.URL.Host)
		}
		var v map[string]any
		url := r.URL.String()
		switch url {
		case latestURL:
			v = map[string]any{"tag_name": "v0.28.0", "body": body, "published_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}
		case hubPrefix + "v0.28.0":
			v = map[string]any{"name": "v0.28.0", "digest": metadata().Digest, "images": []any{map[string]any{"os": "linux", "architecture": "amd64", "digest": "sha256:" + strings.Repeat("c", 64)}}}
		default:
			t.Fatal("unapproved target", url)
		}
		if change != nil {
			change(url, v)
		}
		raw, _ := json.Marshal(v)
		code := 200
		if url == latestURL && status != 0 {
			code = status
		}
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
	})
	return s
}
func TestReleaseAndHubDigestVerification(t *testing.T) {
	s := fixtureClient(t, nil, 0)
	v, e := s.fetch(t.Context())
	if e != nil || v.Version != "0.28.0" || v.URL != "https://github.com/dingding229/mediatool/releases/tag/v0.28.0" {
		t.Fatal(v, e)
	}
	for _, mutate := range []func(string, map[string]any){
		func(u string, v map[string]any) {
			if u == latestURL {
				v["draft"] = true
			}
		}, func(u string, v map[string]any) {
			if u == latestURL {
				v["prerelease"] = true
			}
		},
		func(u string, v map[string]any) {
			if u == latestURL {
				v["tag_name"] = "v0.29.0-rc.1"
			}
		}, func(u string, v map[string]any) {
			if u == latestURL {
				v["body"] = "just notes with no signed-off metadata"
			}
		},
		func(u string, v map[string]any) {
			if u != latestURL {
				v["digest"] = "sha256:" + strings.Repeat("d", 64)
			}
		},
		func(u string, v map[string]any) {
			if u != latestURL {
				v["images"] = []any{map[string]any{"architecture": "arm64", "os": "linux", "digest": metadata().Digest}}
			}
		},
	} {
		if _, e = fixtureClient(t, mutate, 0).fetch(t.Context()); e == nil {
			t.Fatal("untrusted release accepted")
		}
	}
	for _, code := range []int{404, 403, 429, 500} {
		if _, e = fixtureClient(t, nil, code).fetch(t.Context()); e == nil {
			t.Fatal("source failure accepted", code)
		}
	}
}
func TestMetadataParserRejectsAmbiguityAndTruncation(t *testing.T) {
	body, e := ReleaseBody("变更", metadata())
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range []string{"", body + MetadataStart + "{}\n-->", strings.Replace(body, "\n-->", "{}\n-->", 1), strings.Replace(body, `"schema":79`, `"schema":1`, 1), body[:len(body)-4]} {
		if _, e = parseRelease("v0.28.0", b, time.Now()); e == nil {
			t.Fatal("ambiguous metadata")
		}
	}
	if _, e = parseRelease("v0.28.0", strings.TrimSpace(body), time.Now()); e != nil {
		t.Fatal(e)
	}
}
func TestPublicSourcesCannotRedirect(t *testing.T) {
	s := New(nil, nil)
	s.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != latestURL {
			t.Fatal("redirect followed")
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.invalid"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	if _, e := s.fetch(context.Background()); e == nil {
		t.Fatal("redirect accepted")
	}
}
