package playback

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"media-tool/internal/modules/storage"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Owned bytes and isolated TLS servers exercise streaming, not video decoding
// or real CDN/NAS capacity. The existing fixture transport bypass is test-only.
func TestControlledProxyConcurrentRangeBytesAndCredentialIsolation(t *testing.T) {
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	expected := sha256.Sum256(payload)
	var sourceCalls atomic.Int64
	client, policy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Emby-Token") != "" || r.Header.Get("Cookie") != "" {
			t.Error("client credentials leaked to media source")
		}
		if r.Header.Get("Range") != "bytes=0-1048575" {
			t.Error("Range changed")
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.Header().Set("Content-Range", "bytes 0-1048575/1048576")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Set-Cookie", "source-only=fixture")
		w.WriteHeader(206)
		w.Write(payload)
	})
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/owned?temporary=fixture", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Options{Policy: policy, Client: client, ForceProxy: true})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		engine.Serve(w, r, storage.FileRef{Path: "/115/Media/owned.mkv"})
	}))
	defer app.Close()
	consumer := app.Client()
	consumer.Timeout = 10 * time.Second
	started := time.Now()
	var wg sync.WaitGroup
	var succeeded atomic.Int64
	for n := 0; n < 16; n++ {
		wg.Go(func() {
			for i := 0; i < 6; i++ {
				request, _ := http.NewRequest("GET", app.URL, nil)
				request.Header.Set("Range", "bytes=0-1048575")
				request.Header.Set("Authorization", "Bearer synthetic-client-only")
				request.Header.Set("X-Emby-Token", "synthetic-user-only")
				request.Header.Set("Cookie", "administrator=synthetic")
				response, err := consumer.Do(request)
				if err != nil {
					t.Error("proxy request failed", err)
					continue
				}
				h := sha256.New()
				size, readErr := io.CopyBuffer(h, response.Body, make([]byte, 32<<10))
				response.Body.Close()
				if readErr != nil || response.StatusCode != 206 || size != int64(len(payload)) || string(h.Sum(nil)) != string(expected[:]) || response.Header.Get("Set-Cookie") != "" || response.Header.Get("X-Media-Delivery") != "proxy" {
					t.Error("range response was corrupted, truncated or leaked headers")
					continue
				}
				succeeded.Add(1)
			}
		})
	}
	wg.Wait()
	if succeeded.Load() != 96 || sourceCalls.Load() != 96 {
		t.Fatal("not all proxy requests completed", succeeded.Load(), sourceCalls.Load())
	}
	t.Logf("owned TLS proxy: 96 x 1MiB Range requests, 16 concurrent, SHA256/length/status verified, elapsed=%s; not real-media decoding or CDN throughput", time.Since(started).Round(time.Millisecond))
}
