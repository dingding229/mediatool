package linkcache

import (
	"context"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/cache"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type testResolver struct {
	calls   atomic.Int32
	unknown bool
}

func (r *testResolver) Resolve(context.Context, storage.FileRef) (storage.DownloadLink, error) {
	r.calls.Add(1)
	deadline := time.Now().Add(time.Minute)
	if r.unknown {
		deadline = time.Time{}
	}
	return storage.DownloadLink{URL: "https://cdn.test.fixture/media", ExpiresAt: deadline}, nil
}
func TestRedisContextsExpiryInvalidationAndFallback(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL required for real cache test")
	}
	client, err := cache.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	version := "owned-test-" + time.Now().Format(time.RFC3339Nano)
	source := &testResolver{}
	cached := &Cached{Source: source, Redis: client, Version: func(context.Context, storage.FileRef) (string, error) { return version, nil }}
	ref := storage.FileRef{Path: "/115/OwnedTest.mkv", UserAgent: "user-a", ClientIP: "203.0.113.1"}
	if _, err = cached.Resolve(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err = cached.Resolve(ctx, ref); err != nil || source.calls.Load() != 1 {
		t.Fatal("cache missed", err)
	}
	other := ref
	other.UserAgent = "user-b"
	if _, err = cached.Resolve(ctx, other); err != nil || source.calls.Load() != 2 {
		t.Fatal("UA context reused", err)
	}
	cached.Invalidate(ctx, ref)
	if _, err = cached.Resolve(ctx, ref); err != nil || source.calls.Load() != 3 {
		t.Fatal("invalidation failed", err)
	}
	cached.Invalidate(ctx, ref)
	cached.Invalidate(ctx, other)
	source.unknown = true
	if _, err = cached.Resolve(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err = cached.Resolve(ctx, ref); err != nil || source.calls.Load() != 5 {
		t.Fatal("unknown lifetime cached", err)
	}
	dead, _ := cache.Open("redis://127.0.0.1:9/0")
	defer dead.Close()
	uncached := &Cached{Source: source, Redis: dead}
	if _, err = uncached.Resolve(ctx, ref); err != nil {
		t.Fatal("redis outage blocked delivery", err)
	}
}
