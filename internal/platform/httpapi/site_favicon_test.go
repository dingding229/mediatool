package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	mediatoolsite "github.com/dingding229/mediatool-site"
	"github.com/redis/go-redis/v9"
	"io"
	"media-tool/internal/modules/sites"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type faviconTransport func(*http.Request) (*http.Response, error)

func (f faviconTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFaviconFetchHasNoCredentialsAndRejectsHTMLSVGOrOversize(t *testing.T) {
	icon, _ := mediatoolsite.Icon("hdsky")
	for _, test := range []struct {
		raw    []byte
		status int
		ok     bool
	}{{icon, 200, true}, {[]byte("<html>not an image</html>"), 200, false}, {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 200, false}, {bytes.Repeat([]byte("x"), maxFaviconBytes+1), 200, false}, {icon, 302, false}} {
		client := &http.Client{Transport: faviconTransport(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.String() != "https://tracker.example/favicon.ico" {
				t.Fatal("credential or target drift")
			}
			return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(test.raw))}, nil
		})}
		raw, e := fetchSiteFavicon(context.Background(), client, "https://tracker.example/favicon.ico")
		if (e == nil) != test.ok || test.ok && !bytes.Equal(raw, icon) {
			t.Fatal(test.status, len(test.raw), e)
		}
	}
}
func TestFaviconCacheBinaryValidatorAndNegativeCache(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("isolated Redis required")
	}
	options, e := redis.ParseURL(rawURL)
	if e != nil || options.DB != 15 {
		t.Fatal("Redis DB15 required")
	}
	rc := redis.NewClient(options)
	defer rc.Close()
	ctx := context.Background()
	template, _ := sites.GoTemplate("hdsky")
	key := fmt.Sprintf("media-tool:site-favicon:%x", sha256.Sum256([]byte(template.IconURL)))
	defer rc.Del(ctx, key)
	icon, _ := mediatoolsite.Icon("hdsky")
	if e = rc.Set(ctx, key, icon, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	a := &API{Deps: Dependencies{Redis: rc}}
	r := httptest.NewRequest("GET", "/favicon?url=http://127.0.0.1/private", nil)
	r.SetPathValue("id", "hdsky")
	w := httptest.NewRecorder()
	a.ptTemplateFavicon(w, r)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), icon) || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") || w.Header().Get("Cache-Control") != "private, max-age=86400" {
		t.Fatal(w.Code, w.Header())
	}
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	a.ptTemplateFavicon(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal(w.Code)
	}
	if e = rc.Set(ctx, key, []byte{}, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	a.ptTemplateFavicon(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	r.SetPathValue("id", "unknown-template")
	w = httptest.NewRecorder()
	a.ptTemplateFavicon(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
