package runtime

import (
	"context"
	"errors"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/storage"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sourceFunc func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f sourceFunc) Resolve(c context.Context, r storage.FileRef) (storage.DownloadLink, error) {
	return f(c, r)
}
func TestSourceCooldownAndBoundedLanes(t *testing.T) {
	var calls atomic.Int32
	l := &limitedSource{source: sourceFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		calls.Add(1)
		return storage.DownloadLink{}, open115.ErrRateLimited
	}), lanes: make(chan struct{}, 4)}
	for i := 0; i < 3; i++ {
		_, err := l.Resolve(t.Context(), storage.FileRef{})
		var retry *nativeplayback.RetryError
		if !errors.As(err, &retry) || retry.After <= 0 {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cooldown hammered provider", calls.Load())
	}
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var active, peak atomic.Int32
	l = &limitedSource{source: sourceFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		entered <- struct{}{}
		select {
		case <-release:
			return storage.DownloadLink{}, nil
		case <-ctx.Done():
			return storage.DownloadLink{}, ctx.Err()
		}
	}), lanes: make(chan struct{}, 4)}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Resolve(t.Context(), storage.FileRef{}); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-entered
	}
	wait, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := l.Resolve(wait, storage.FileRef{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unbounded queue", err)
	}
	close(release)
	wg.Wait()
	if peak.Load() != 4 {
		t.Fatal(peak.Load())
	}
}
