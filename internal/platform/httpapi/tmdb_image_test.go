package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"image"
	"image/png"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTMDBImageFixedOriginSignatureAndNoCredentialForwarding(t *testing.T) {
	var pixel bytes.Buffer
	png.Encode(&pixel, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	calls := 0
	manager := &integrations.Manager{OutboundHTTP: &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "image.tmdb.org" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "" {
			t.Error("credentials/URL forwarded")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(pixel.Bytes()))}, nil
	})}}
	a := API{Deps: Dependencies{Integrations: manager}}
	query := func(size, file string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/image", nil)
		r.Header.Set("Authorization", "must-not-forward")
		r.Header.Set("Cookie", "private")
		r.Header.Set("User-Agent", "private-agent")
		r.SetPathValue("size", size)
		r.SetPathValue("file", file)
		w := httptest.NewRecorder()
		a.tmdbImage(w, r)
		return w
	}
	w := query("w500", "owned.png")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, test := range [][2]string{{"original", "owned.png"}, {"w500", "../private.png"}, {"w500", "https://bad.png"}, {"w500", "owned.svg"}} {
		if query(test[0], test[1]).Code != 404 {
			t.Fatal(test)
		}
	}
	if calls != 1 {
		t.Fatal("invalid paths contacted provider")
	}
	manager.OutboundHTTP = &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewBufferString("<html>challenge</html>"))}, nil
	})}
	if query("w500", "owned.png").Code != 502 {
		t.Fatal("HTML returned as image")
	}
}

func TestRP34TMDBImageAdmissionIsBounded(t *testing.T) {
	var b tmdbImageAdmission
	now := time.Unix(600, 0)
	var leaves []func()
	for i := 0; i < tmdbImageConcurrent; i++ {
		leave, _ := b.enter("source", now)
		if leave == nil {
			t.Fatal("slot not admitted", i)
		}
		leaves = append(leaves, leave)
	}
	if leave, retry := b.enter("different-source", now); leave != nil || retry != "1" {
		t.Fatal("global limit not enforced")
	}
	for _, leave := range leaves {
		leave()
	}
	for i := len(leaves); i < tmdbImagePerSource; i++ {
		leave, _ := b.enter("source", now)
		if leave == nil {
			t.Fatal("source rejected early", i)
		}
		leave()
	}
	if leave, retry := b.enter("source", now); leave != nil || retry != "60" {
		t.Fatal("source quota not enforced")
	}
	if leave, _ := b.enter("different-source", now); leave == nil {
		t.Fatal("one source consumed another's quota")
	} else {
		leave()
	}
	if leave, _ := b.enter("source", now.Add(time.Minute)); leave == nil {
		t.Fatal("source quota did not reset")
	} else {
		leave()
	}
}

func TestRP34TMDBImageCacheHitDoesNotRefreshTTL(t *testing.T) {
	if os.Getenv("TEST_REDIS_URL") == "" {
		t.Skip("isolated Redis required")
	}
	opts, err := redis.ParseURL(os.Getenv("TEST_REDIS_URL"))
	if err != nil || opts.DB != 15 {
		t.Fatal("Redis DB15 required")
	}
	rc := redis.NewClient(opts)
	t.Cleanup(func() { _ = rc.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file := jobs.UUID() + ".png"
	key := fmt.Sprintf("media-tool:tmdb-image-cache:%x", sha256.Sum256([]byte("w500/"+file)))
	defer rc.Del(ctx, key)
	a := &API{Deps: Dependencies{Redis: rc}}
	var pngBytes bytes.Buffer
	if err = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err = rc.Set(ctx, key, pngBytes.Bytes(), time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/image", nil).WithContext(ctx)
	r.SetPathValue("size", "w500")
	r.SetPathValue("file", file)
	w := httptest.NewRecorder()
	a.tmdbImage(w, r)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pngBytes.Bytes()) {
		t.Fatal(w.Code)
	}
	if ttl, err := rc.PTTL(ctx, key).Result(); err != nil || ttl > time.Minute || ttl < 0 {
		t.Fatal("cache hit rewrote expiry", ttl, err)
	}
}

func TestRP34TMDBImageSameKeySharesOneFetch(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	start, release := make(chan struct{}), make(chan struct{})
	manager := &integrations.Manager{OutboundHTTP: &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(start)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data.Bytes()))}, nil
	})}}
	a := &API{Deps: Dependencies{Integrations: manager}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := a.thumbnailFlight(ctx, "w500", "owned.png")
	select {
	case <-start:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream not contacted")
	}
	second := a.thumbnailFlight(context.Background(), "w500", "owned.png")
	cancel() // The fetch belongs to both clients, not the first caller's lifetime.
	close(release)
	for _, c := range []<-chan singleflight.Result{first, second} {
		select {
		case result := <-c:
			if result.Err != nil || !result.Shared || !bytes.Equal(result.Val.(tmdbThumbnail).body, data.Bytes()) {
				t.Fatal("fetch not shared", result.Err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("coalesced fetch not completed")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate upstream requests", calls.Load())
	}
}
