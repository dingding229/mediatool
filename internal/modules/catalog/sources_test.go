package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/redis/go-redis/v9"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSources struct {
	snapshot string
	enabled  bool
	calls    atomic.Int32
	result   contracts.SourceResult
	err      error
	wait     chan struct{}
	entered  chan struct{}
}

func (f *fakeSources) SourceProviders(_ context.Context, kind contracts.ChainEventType) ([]pluginv2.Snapshot, error) {
	if !f.enabled || kind != contracts.ChainDiscoverSource {
		return nil, nil
	}
	return []pluginv2.Snapshot{{ID: f.snapshot, InstanceID: "f5a-source-fixture"}}, nil
}
func (f *fakeSources) SourcePage(ctx context.Context, _ pluginv2.Snapshot, _ contracts.ChainEventType, _ contracts.SourceRequest) (contracts.SourceResult, error) {
	f.calls.Add(1)
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.wait != nil {
		select {
		case <-f.wait:
		case <-ctx.Done():
			return contracts.SourceResult{}, ctx.Err()
		}
	}
	return f.result, f.err
}
func (f *fakeSources) Descriptor(context.Context, string, string, string) (pluginv2.Snapshot, contracts.Descriptor, error) {
	if !f.enabled {
		return pluginv2.Snapshot{}, contracts.Descriptor{}, pluginv2.ErrUnavailable
	}
	return pluginv2.Snapshot{ID: f.snapshot}, contracts.Descriptor{}, nil
}
func sourceFixture(t *testing.T) (SourceRegistry, *fakeSources, *time.Time) {
	t.Helper()
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("Redis DB15 required")
	}
	o, e := redis.ParseURL(u)
	if e != nil || o.DB != 15 {
		t.Fatal("only Redis DB15 permitted")
	}
	rc := redis.NewClient(o)
	if e = rc.Ping(context.Background()).Err(); e != nil {
		t.Fatal(e)
	}
	keys := []string{sourceKey("f5a-source-fixture", "discover.source"), "plugins:v2:sources:lease"}
	rc.Del(context.Background(), keys...)
	t.Cleanup(func() { rc.Del(context.Background(), keys...); rc.Close() })
	now := time.Now().UTC()
	f := &fakeSources{snapshot: "generation-one", enabled: true, result: contracts.SourceResult{Items: []contracts.SourceCandidate{{MediaIdentity: contracts.MediaIdentity{TMDBID: 1, MediaType: "movie"}, Title: "One"}}}}
	return SourceRegistry{Store: f, Cache: rc, Now: func() time.Time { return now }}, f, &now
}
func TestSourceFreshnessAndSnapshotInvalidation(t *testing.T) {
	s, f, now := sourceFixture(t)
	ctx := context.Background()
	refresh := func() {
		t.Helper()
		if e := s.Refresh(ctx); e != nil {
			t.Fatal(e)
		}
	}
	refresh()
	for range 7 {
		*now = now.Add(15 * time.Second)
		refresh()
	}
	if f.calls.Load() != 1 {
		t.Fatal("TTL ignored", f.calls.Load())
	}
	p, e := s.Page(ctx, "discover.source", 1, 200)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	if f.calls.Load() != 1 {
		t.Fatal("page invoked worker")
	}
	*now = now.Add(15 * time.Second)
	p, _ = s.Page(ctx, "discover.source", 1, 200)
	if len(p.Items) != 0 {
		t.Fatal("stale cards retained")
	}
	refresh()
	if f.calls.Load() != 2 {
		t.Fatal("due source not refreshed")
	}
	f.enabled = false
	p, _ = s.Page(ctx, "discover.source", 1, 200)
	if len(p.Items) != 0 {
		t.Fatal("disabled/revoked provider remained visible")
	}
	refresh()
	if f.calls.Load() != 2 {
		t.Fatal("disabled invoked")
	}
	f.enabled = true
	f.snapshot = "generation-two"
	p, _ = s.Page(ctx, "discover.source", 1, 200)
	if len(p.Items) != 0 {
		t.Fatal("old generation cache reused")
	}
	refresh()
	if f.calls.Load() != 3 {
		t.Fatal("new config suppressed by old TTL")
	}
}
func TestSourceEmptyFailureBackoffBoundedAndRecovery(t *testing.T) {
	s, f, now := sourceFixture(t)
	ctx := context.Background()
	f.result = contracts.SourceResult{Items: []contracts.SourceCandidate{}}
	for _, delay := range []time.Duration{120, 240, 480, 900, 900, 900, 900} {
		if e := s.Refresh(ctx); e != nil {
			t.Fatal(e)
		}
		v, ok := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot)
		if !ok || v.NextAt.Sub(*now) != delay*time.Second || v.EmptyStreak > 6 {
			t.Fatal(v, ok)
		}
		calls := f.calls.Load()
		*now = v.NextAt.Add(-time.Second)
		s.Refresh(ctx)
		if f.calls.Load() != calls {
			t.Fatal("empty results hot polled")
		}
		*now = v.NextAt
	}
	f.err = errors.New("provider failure")
	for _, delay := range []time.Duration{30, 60, 120, 240, 300, 300, 300} {
		if e := s.Refresh(ctx); e != nil {
			t.Fatal(e)
		}
		v, ok := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot)
		if !ok || v.NextAt.Sub(*now) != delay*time.Second || v.Failures > 6 || !v.FreshUntil.IsZero() {
			t.Fatal(v, ok)
		}
		calls := f.calls.Load()
		*now = v.NextAt.Add(-time.Second)
		s.Refresh(ctx)
		if f.calls.Load() != calls {
			t.Fatal("failed source hot polled")
		}
		*now = v.NextAt
	}
	f.err = nil
	f.result = contracts.SourceResult{Items: []contracts.SourceCandidate{{MediaIdentity: contracts.MediaIdentity{TMDBID: 2, MediaType: "movie"}, Title: "Recovered"}}}
	s.Refresh(ctx)
	v, _ := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot)
	if v.Failures != 0 || v.EmptyStreak != 0 || v.NextAt.Sub(*now) != sourceFresh {
		t.Fatal(v)
	}
	// Invalid provider responses are failures, never published as cards.
	*now = v.NextAt
	f.result.Items[0].TMDBID = 0
	s.Refresh(ctx)
	p, _ := s.Page(ctx, "discover.source", 1, 200)
	if len(p.Items) != 1 || p.Items[0].TMDBID != 2 {
		t.Fatal("invalid result replaced last valid cards", p)
	}
	// A new generation skips a failed generation's negative cache immediately.
	f.snapshot = "generation-new"
	before := f.calls.Load()
	s.Refresh(ctx)
	if f.calls.Load() != before+1 {
		t.Fatal("negative cache crossed generation")
	}
}
func TestSourceConcurrentRefreshLeaseAndOldCache(t *testing.T) {
	s, f, _ := sourceFixture(t)
	ctx := context.Background()
	f.entered = make(chan struct{}, 1)
	f.wait = make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := s.Refresh(ctx); e != nil {
			t.Error(e)
		}
	}()
	<-f.entered
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if f.calls.Load() != 1 {
		t.Fatal("lease did not coalesce")
	}
	close(f.wait)
	wg.Wait()
	f.wait = nil
	f.entered = nil
	for _, raw := range []string{`{"snapshot":"generation-one","result":{"items":[]}}`, `{"version":2,"unknown":true}`, `{`} {
		s.Cache.Set(ctx, sourceKey("f5a-source-fixture", "discover.source"), raw, time.Minute)
		if _, ok := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot); ok {
			t.Fatal("old/malformed envelope used")
		}
	}
	s.Refresh(ctx)
	v, _ := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot)
	v.NextAt = v.WrittenAt.Add(time.Hour)
	raw, _ := json.Marshal(v)
	s.Cache.Set(ctx, sourceKey("f5a-source-fixture", "discover.source"), raw, time.Minute)
	if _, ok := s.cached(ctx, "f5a-source-fixture", "discover.source", f.snapshot); ok {
		t.Fatal("unbounded retry accepted")
	}
}

func TestSourceFailureKeepsLastSuccessOnlyWithinSevenMinutes(t *testing.T) {
	s, f, now := sourceFixture(t)
	ctx := context.Background()
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	success := *now
	*now = now.Add(120 * time.Second)
	f.err = errors.New("offline")
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.Page(ctx, "discover.source", 1, 200)
	if e != nil || len(p.Items) != 1 {
		t.Fatal("valid old cards disappeared", p, e)
	}
	*now = success.Add(419 * time.Second)
	s.Refresh(ctx)
	p, e = s.Page(ctx, "discover.source", 1, 200)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	*now = success.Add(420 * time.Second)
	p, e = s.Page(ctx, "discover.source", 1, 200)
	if e != nil || len(p.Items) != 0 {
		t.Fatal("stale cards exceeded cap", p, e)
	}
	f.snapshot = "new-config"
	p, e = s.Page(ctx, "discover.source", 1, 200)
	if e != nil || len(p.Items) != 0 {
		t.Fatal("stale data crossed generation", p, e)
	}
}
