package search

import (
	"context"
	"fmt"
	"media-tool/internal/modules/metadata"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func searchAccountIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("account-%d", i)
	}
	return ids
}

func TestSearchAccountLimitIncludesAllEnabledAccounts(t *testing.T) {
	for _, count := range []int{1, 10, 11, 200} {
		if err := validateAccountIDs(searchAccountIDs(count)); err != nil {
			t.Fatalf("%d enabled accounts rejected: %v", count, err)
		}
	}
	for _, ids := range [][]string{nil, searchAccountIDs(201), {"same", "same"}} {
		if err := validateAccountIDs(ids); err == nil {
			t.Fatalf("invalid account list accepted: %v", ids)
		}
	}
}

func TestSearchAccountLimitsApplyBeforeCachedLookups(t *testing.T) {
	engine := Engine{}
	for _, ids := range [][]string{nil, searchAccountIDs(201), {"same", "same"}} {
		if _, _, err := engine.StartCached(context.Background(), "Owned", ids, false); err == nil {
			t.Fatal("invalid cached search account scope accepted")
		}
		if _, _, err := engine.StartMediaCached(context.Background(), "Owned", ids, false, metadata.Item{ID: 1, MediaType: "movie"}); err == nil {
			t.Fatal("invalid media cached search account scope accepted")
		}
	}
}

func TestSearchAccountConcurrencyIsBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, release, done := make(chan struct{}, 200), make(chan struct{}), make(chan struct{})
	var running, peak atomic.Int32
	var mu sync.Mutex
	visited := map[string]int{}
	go func() {
		defer close(done)
		forEachAccount(ctx, searchAccountIDs(200), func(id string) {
			n := running.Add(1)
			defer running.Add(-1)
			for p := peak.Load(); n > p; p = peak.Load() {
				if peak.CompareAndSwap(p, n) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			mu.Lock()
			visited[id]++
			mu.Unlock()
		})
	}()
	for range searchConcurrency {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("workers did not finish")
	}
	if peak.Load() != 3 || len(visited) != 200 {
		t.Fatalf("peak=%d visited=%d", peak.Load(), len(visited))
	}
	for id, count := range visited {
		if count != 1 {
			t.Fatalf("%s visited %d times", id, count)
		}
	}
}

func TestSearchAccountCancellationStopsScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}, 200), make(chan struct{})
	go func() {
		defer close(done)
		forEachAccount(ctx, searchAccountIDs(200), func(string) {
			started <- struct{}{}
			<-ctx.Done()
		})
	}()
	for range searchConcurrency {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled workers did not finish")
	}
	if len(started) != 0 {
		t.Fatal("new work scheduled after cancellation")
	}
}
