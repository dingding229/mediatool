package httpapi

import (
	"context"
	"errors"
	"io"
	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/nativelibrary"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeImageSizeMappingAndPathValidation(t *testing.T) {
	for _, tc := range []struct {
		width int
		want  string
	}{{0, "w500"}, {1, "w92"}, {92, "w92"}, {93, "w154"}, {342, "w342"}, {343, "w500"}, {1281, "original"}} {
		if got := imageSize(tc.width); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	for _, path := range []string{"/poster.jpg", "/poster.png", "/poster.webp"} {
		if !validTMDBImagePath(path) {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"https://evil.test/a.jpg", "/../a.jpg", "/a?x=1", "/a.txt", "pickcode.jpg"} {
		if validTMDBImagePath(path) {
			t.Fatal("unsafe path accepted", path)
		}
	}
	if nativelibrary.ImageTag("/poster.jpg") != nativelibrary.ImageTag("/poster.jpg") {
		t.Fatal("unstable image tag")
	}
}
func TestNativeImageFetchSingleflightCacheAndLimits(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0})
	}))
	defer server.Close()
	resolver := &NativeImageResolver{ImageBaseURL: server.URL}
	results := make(chan []byte, 20)
	for i := 0; i < 20; i++ {
		go func() {
			raw, _, err := resolver.load(context.Background(), "w342", "/poster.png")
			if err == nil {
				results <- raw
			}
		}()
	}
	for i := 0; i < 20; i++ {
		select {
		case raw := <-results:
			if len(raw) == 0 {
				t.Fatal("empty")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("singleflight timeout")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("fetches not merged", calls.Load())
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", 4<<20+1)))
	}))
	defer big.Close()
	if _, _, err := (&NativeImageResolver{ImageBaseURL: big.URL}).load(context.Background(), "w342", "/big.png"); !errors.Is(err, embyapi.ErrImageUnavailable) {
		t.Fatal(err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, _, err := (&NativeImageResolver{ImageBaseURL: slow.URL}).load(ctx, "w342", "/slow.png"); err == nil {
		t.Fatal("timeout accepted")
	}
}
