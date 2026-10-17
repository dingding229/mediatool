package settings

import (
	"context"
	"errors"
	"media-tool/internal/modules/nativelibrary"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeServerCacheExpiryDisableAndNoStaleOnError(t *testing.T) {
	var tick atomic.Int64
	var calls atomic.Int32
	value := nativelibrary.ServerSettings{Enabled: true, ServerName: "Synthetic", Revision: 1}
	var failure error
	repo := &NativeServerRepository{now: func() time.Time { return time.Unix(0, tick.Load()) }, read: func(ctx context.Context) (nativelibrary.ServerSettings, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > nativeSettingsTimeout {
			t.Error("unbounded read")
		}
		return value, failure
	}}
	first, err := repo.Get(context.Background())
	if err != nil || !first.Enabled {
		t.Fatal(first, err)
	}
	value.Enabled = false
	value.Revision = 2
	cached, err := repo.Get(context.Background())
	if err != nil || !cached.Enabled || calls.Load() != 1 {
		t.Fatal("cache not used", cached, err)
	}
	tick.Store(int64(NativeServerCacheTTL + time.Nanosecond))
	disabled, err := repo.Get(context.Background())
	if err != nil || disabled.Enabled || disabled.Revision != 2 || calls.Load() != 2 {
		t.Fatal("disable not refreshed", disabled, err)
	}
	failure = errors.New("private SQL failure")
	tick.Add(int64(NativeServerCacheTTL + time.Nanosecond))
	stale, err := repo.Get(context.Background())
	if !errors.Is(err, nativelibrary.ErrSettingsUnavailable) || stale.Enabled || stale.ServerName != "" {
		t.Fatal("stale snapshot served", stale, err)
	}
}
func TestNativeServerCacheConcurrentRefreshAndCancellation(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	repo := &NativeServerRepository{now: time.Now, read: func(ctx context.Context) (nativelibrary.ServerSettings, error) {
		calls.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return nativelibrary.ServerSettings{ServerName: "Synthetic"}, nil
		case <-ctx.Done():
			return nativelibrary.ServerSettings{}, ctx.Err()
		}
	}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			v, err := repo.Get(context.Background())
			if err != nil || v.ServerName != "Synthetic" {
				t.Error(v, err)
			}
		})
	}
	<-entered
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.Get(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("concurrent cache miss multiplied queries", calls.Load())
	}
	for _, r := range []*NativeServerRepository{nil, {}, NewNativeServerRepository(nil)} {
		if _, err := r.Get(context.Background()); !errors.Is(err, nativelibrary.ErrSettingsUnavailable) {
			t.Fatal(err)
		}
	}
}

func TestNativeServerCacheCanceledReadReleasesBackgroundRefresh(t *testing.T) {
	entered, done := make(chan struct{}), make(chan struct{})
	repo := &NativeServerRepository{now: time.Now, read: func(ctx context.Context) (nativelibrary.ServerSettings, error) {
		defer close(done)
		close(entered)
		<-ctx.Done()
		return nativelibrary.ServerSettings{}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := repo.Get(ctx); result <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled refresh remained running")
	}
	if repo.cached.Load() != nil {
		t.Fatal("failed refresh was cached")
	}
}
