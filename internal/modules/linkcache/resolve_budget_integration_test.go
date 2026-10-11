package linkcache

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/cache"
)

type resolveFixture func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f resolveFixture) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	return f(ctx, ref)
}
func isolatedCache(t *testing.T) *Cached {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("isolated Redis DB15 required")
	}
	client, err := cache.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return &Cached{Redis: client, Version: func(context.Context, storage.FileRef) (string, error) { return t.Name(), nil }}
}
func TestCacheSharedWaiterCancellationDoesNotCancelRemainingWaiter(t *testing.T) {
	c := isolatedCache(t)
	c.ResolveTimeout = 500 * time.Millisecond
	started, released := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c.Source = resolveFixture(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		calls.Add(1)
		close(started)
		select {
		case <-released:
			return storage.DownloadLink{URL: "https://cdn.fixture/owned", CacheUntil: time.Now().Add(time.Minute)}, nil
		case <-ctx.Done():
			return storage.DownloadLink{}, ctx.Err()
		}
	})
	ref := storage.FileRef{Path: "/owned/" + t.Name(), UserAgent: "one-UA", ClientIP: "203.0.113.1"}
	first, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() { _, err := c.Resolve(first, ref); firstDone <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("source did not start")
	}
	secondDone := make(chan error, 1)
	secondCtx, secondCancel := context.WithTimeout(context.Background(), time.Second)
	defer secondCancel()
	waiting := make(chan struct{}, 1)
	secondCtx = storage.WithResolveObserver(secondCtx, func(e storage.ResolveObservation) {
		if e.Stage == storage.StageCacheJoined {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
	})
	go func() { _, err := c.Resolve(secondCtx, ref); secondDone <- err }()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("second waiter did not join")
	}
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled caller kept waiting", err)
	}
	close(released)
	if err := <-secondDone; err != nil {
		t.Fatal("one cancelled client terminated another caller", err)
	}
	if calls.Load() != 1 {
		t.Fatal("shared source request duplicated")
	}
	c.Invalidate(context.Background(), ref)
}
func TestCacheQueueAndSourceBudgetsAreBoundedAndObservable(t *testing.T) {
	for _, queued := range []bool{true, false} {
		t.Run(map[bool]string{true: "queue", false: "source"}[queued], func(t *testing.T) {
			c := isolatedCache(t)
			c.ResolveTimeout = 50 * time.Millisecond
			var calls atomic.Int32
			c.Source = resolveFixture(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
				calls.Add(1)
				<-ctx.Done()
				return storage.DownloadLink{}, ctx.Err()
			})
			if queued {
				c.once.Do(func() { c.gate = make(chan struct{}, 4) })
				for i := 0; i < 4; i++ {
					c.gate <- struct{}{}
				}
			}
			var mu sync.Mutex
			events := []storage.ResolveObservation{}
			ctx := storage.WithResolveObserver(context.Background(), func(e storage.ResolveObservation) { mu.Lock(); defer mu.Unlock(); events = append(events, e) })
			started := time.Now()
			_, err := c.Resolve(ctx, storage.FileRef{Path: "/owned/" + t.Name()})
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
				t.Fatal("unbounded/lost deadline", err)
			}
			mu.Lock()
			defer mu.Unlock()
			wanted := storage.StageSourceResolve
			if queued {
				wanted = storage.StageSourceQueue
				if calls.Load() != 0 {
					t.Fatal("queued source executed after its budget")
				}
			}
			found := false
			for _, e := range events {
				if e.Stage == wanted && e.Outcome == "timeout" {
					found = true
				}
			}
			if !found {
				t.Fatal("missing source/queue phase", events)
			}
		})
	}
}
func TestAlreadyCancelledCacheRequestCannotLaunchDetachedProviderWork(t *testing.T) {
	c := isolatedCache(t)
	var calls atomic.Int32
	c.Source = resolveFixture(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		calls.Add(1)
		return storage.DownloadLink{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Resolve(ctx, storage.FileRef{Path: "/owned"}); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatal("cancelled caller launched provider work", err)
	}
}
