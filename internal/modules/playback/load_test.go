package playback

import (
	"context"
	"io"
	"media-tool/internal/modules/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Replaces the retired positive relay throughput test: arbitrary Range/Cookie
// inputs must never make this server read or relay media for a non-DirectOnly link.
func TestStrictRedirectConcurrentRangesNeverReadOrRelayBytes(t *testing.T) {
	var sourceCalls atomic.Int64
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		t.Error("server fetched media for non-official link")
	})
	e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/original?temporary=fixture"}, nil
	}), Options{Policy: p, Client: client, ProxyFallback: true})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.Serve(w, r, storage.FileRef{}) }))
	defer app.Close()
	consumer := app.Client()
	consumer.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var passed atomic.Int64
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 6 {
				r, _ := http.NewRequest("GET", app.URL, nil)
				r.Header.Set("Range", "bytes=0-1048575")
				r.Header.Set("Authorization", "Bearer synthetic-only")
				r.Header.Set("Cookie", "admin=synthetic-only")
				response, err := consumer.Do(r)
				if err != nil {
					t.Error(err)
					continue
				}
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 302 || response.Header.Get("X-Media-Delivery") != "redirect" || response.Header.Get("Location") != "https://cdn.example.com/original?temporary=fixture" || len(body) > 1024 || strings.Contains(string(body), "synthetic-only") {
					t.Error(response.StatusCode, response.Header, err)
					continue
				}
				passed.Add(1)
			}
		})
	}
	wg.Wait()
	if passed.Load() != 96 || sourceCalls.Load() != 0 {
		t.Fatal("relay or missing redirects", passed.Load(), sourceCalls.Load())
	}
}
func TestStrictDeliveryRejectsMethodsAndHeadersBeforeAnyMediaRead(t *testing.T) {
	for _, official := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			var reads atomic.Int32
			client, p := proxyFixture(t, func(http.ResponseWriter, *http.Request) { reads.Add(1) })
			e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
				return storage.DownloadLink{URL: "https://cdn.example.com/original", DirectOnly: official, Headers: map[string]string{"Cookie": "provider-secret"}}, nil
			}), Options{Policy: p, Client: client, ForceProxy: force, ProxyFallback: true})
			for _, method := range []string{"GET", "HEAD", "POST", "PUT", "DELETE"} {
				w := httptest.NewRecorder()
				e.Serve(w, httptest.NewRequest(method, "http://gateway/original", nil), storage.FileRef{})
				want := 409
				if method != "GET" && method != "HEAD" {
					want = 405
				}
				if w.Code != want || w.Header().Get("Location") != "" || w.Header().Get("X-Media-Delivery") == "proxy" || strings.Contains(w.Body.String(), "STRM_PROXY_FALLBACK") || strings.Contains(w.Body.String(), "provider-secret") {
					t.Fatal(method, w.Code, w.Body.String())
				}
			}
			if reads.Load() != 0 {
				t.Fatal("media bytes read", reads.Load())
			}
		}
	}
}
