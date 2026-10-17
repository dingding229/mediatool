//go:build linux || darwin

package stillstore

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/mediaprobe/stillqueue"
)

type fakeCacheBackend struct {
	mu            sync.Mutex
	cfg           stillqueue.Config
	snapshots     map[int64]stillqueue.Snapshot
	active        map[string]bool
	deny          bool
	evicted       []stillqueue.Snapshot
	touches       int
	maxBatch      int
	referenceHook func(context.Context, string) (bool, error)
}

func newFakeCache() *fakeCacheBackend {
	return &fakeCacheBackend{snapshots: map[int64]stillqueue.Snapshot{}, active: map[string]bool{}}
}
func (f *fakeCacheBackend) Limits() stillqueue.Config { return f.cfg }
func (f *fakeCacheBackend) SnapshotPage(_ context.Context, after int64, unknown bool, limit int) ([]stillqueue.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []stillqueue.Snapshot
	for id, s := range f.snapshots {
		if id > after && (!unknown || s.SizeBytes == 0) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (f *fakeCacheBackend) Oldest(ctx context.Context, limit int) ([]stillqueue.Snapshot, error) {
	rows, err := f.SnapshotPage(ctx, 0, false, 200)
	sort.Slice(rows, func(i, j int) bool { return rows[i].LastAccessAt.Before(rows[j].LastAccessAt) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, err
}
func (f *fakeCacheBackend) Admit(_ context.Context, _ stillqueue.Lease, _ int64) (stillqueue.Admission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.snapshots {
		if s.SizeBytes == 0 {
			return stillqueue.Admission{}, stillqueue.ErrUnknownSize
		}
	}
	for _, s := range f.evicted {
		delete(f.snapshots, s.ItemID)
	} // Simulate committed reference removal.
	out := stillqueue.Admission{Reserved: !f.deny, Evicted: f.evicted}
	f.evicted = nil
	return out, nil
}
func (f *fakeCacheBackend) Calibrate(_ context.Context, s stillqueue.Snapshot, size int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.snapshots[s.ItemID]
	if !ok || old.Digest != s.Digest || old.RelativePath != s.RelativePath || !old.PublishedAt.Equal(s.PublishedAt) {
		return false, nil
	}
	if size == 0 {
		delete(f.snapshots, s.ItemID)
	} else {
		old.SizeBytes = size
		f.snapshots[s.ItemID] = old
	}
	return true, nil
}
func (f *fakeCacheBackend) EvictSnapshot(ctx context.Context, s stillqueue.Snapshot) (bool, error) {
	return f.Calibrate(ctx, s, 0)
}
func (f *fakeCacheBackend) Referenced(ctx context.Context, path string) (bool, error) {
	if f.referenceHook != nil {
		return f.referenceHook(ctx, path)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active[path] {
		return true, nil
	}
	for _, s := range f.snapshots {
		if s.RelativePath == path {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeCacheBackend) TouchBatch(_ context.Context, a []stillqueue.Access) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touches++
	f.maxBatch = max(f.maxBatch, len(a))
	return int64(len(a)), nil
}
func (f *fakeCacheBackend) Usage(context.Context) (u stillqueue.Usage, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.snapshots {
		u.SnapshotBytes += s.SizeBytes
		if s.SizeBytes == 0 {
			u.UnknownSizes++
		}
	}
	return u, nil
}
func testManagedCache(t *testing.T, f *fakeCacheBackend) (*Cache, string) {
	t.Helper()
	s, root := testStore(t)
	c, err := newCache(s, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c, root
}
func cacheLease() stillqueue.Lease {
	return stillqueue.Lease{ItemID: 100001, AssetID: testAsset, Fingerprint: testFP, Token: "aa111111-1111-4111-8111-111111111111", Attempt: 1}
}
func snapshotImage(t *testing.T, c *Cache, id int64, asset string) stillqueue.Snapshot {
	t.Helper()
	image, err := c.store.Publish(t.Context(), asset, testFP, testJPEG(t, color.Black, 16, 9))
	if err != nil {
		t.Fatal(err)
	}
	return stillqueue.Snapshot{Image: image, ItemID: id, PublishedAt: time.Now().Add(-2 * time.Hour), LastAccessAt: time.Now().Add(-2 * time.Hour)}
}
func ageFile(t *testing.T, root, path string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, path), old, old); err != nil {
		t.Fatal(err)
	}
}
func drainGC(t *testing.T, c *Cache, limit int) {
	t.Helper()
	for i := 0; i < 600; i++ {
		r, err := c.GC(t.Context(), limit)
		if err != nil {
			t.Fatal(err)
		}
		if r.Visited > limit {
			t.Fatal("unbounded visit", r)
		}
		if r.Done {
			return
		}
	}
	t.Fatal("GC cursor did not complete")
}
func TestCacheUnknownBadStatDoesNotBlockPublication(t *testing.T) {
	f := newFakeCache()
	c, root := testManagedCache(t, f)
	bad := snapshotImage(t, c, 100010, "cd222222-2222-4222-8222-222222222222")
	outside := filepath.Join(realTemp(t), "sentinel")
	if err := os.WriteFile(outside, []byte("synthetic sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, bad.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, bad.RelativePath)); err != nil {
		t.Fatal(err)
	}
	bad.SizeBytes = 0
	f.snapshots[bad.ItemID] = bad
	image, err := c.Publish(t.Context(), cacheLease(), testJPEG(t, color.Black, 16, 9))
	if err != nil || image.SizeBytes == 0 {
		t.Fatal("bad stat globally blocked publisher", err)
	}
	if len(f.snapshots) != 0 || c.CleanupFailures() == 0 {
		t.Fatal("bad reference not retired")
	}
	raw, err := os.ReadFile(outside)
	if err != nil || string(raw) != "synthetic sentinel" {
		t.Fatal("followed external symlink", err)
	}
}
func TestCacheUnknownBatchRetriesThenProgresses(t *testing.T) {
	f := newFakeCache()
	c, _ := testManagedCache(t, f)
	for i := 0; i < 201; i++ {
		f.snapshots[int64(100100+i)] = stillqueue.Snapshot{ItemID: int64(100100 + i), Image: stillqueue.Image{RelativePath: "legacy/not-a-controlled-file"}, PublishedAt: time.Now()}
	}
	_, err := c.Publish(t.Context(), cacheLease(), testJPEG(t, color.Black, 16, 9))
	var retry *RetryError
	if !errors.As(err, &retry) || retry.After <= 0 || len(f.snapshots) != 1 {
		t.Fatal("missing bounded retry", len(f.snapshots), err)
	}
	if _, err := c.Publish(t.Context(), cacheLease(), testJPEG(t, color.Black, 16, 9)); err != nil {
		t.Fatal("sticky global refusal", err)
	}
}
func TestCacheCommitThenDeleteAndCapacityRetry(t *testing.T) {
	f := newFakeCache()
	c, root := testManagedCache(t, f)
	old := snapshotImage(t, c, 100010, "cd222222-2222-4222-8222-222222222222")
	f.snapshots[old.ItemID] = old
	f.evicted = []stillqueue.Snapshot{old}
	f.deny = true
	f.referenceHook = func(_ context.Context, path string) (bool, error) {
		if _, ok := f.snapshots[old.ItemID]; ok {
			t.Error("unlink check preceded commit")
		}
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Error("file removed before fresh reference check")
		}
		return false, nil
	}
	_, err := c.Publish(t.Context(), cacheLease(), testJPEG(t, color.Black, 16, 9))
	var retry *RetryError
	if !errors.As(err, &retry) || retry.Reason != stillqueue.StorageFull {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, old.RelativePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("committed victim not deleted", err)
	}
	f.referenceHook = nil
	f.deny = false
	if _, err := c.Publish(t.Context(), cacheLease(), testJPEG(t, color.Black, 16, 9)); err != nil {
		t.Fatal(err)
	}
}
func TestCacheReadsCoalesceAndMissingFilesFallBack(t *testing.T) {
	f := newFakeCache()
	c, root := testManagedCache(t, f)
	s := snapshotImage(t, c, 100001, testAsset)
	f.snapshots[s.ItemID] = s
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if raw, err := c.Read(t.Context(), s); err != nil || len(raw) == 0 {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.touches != 0 || len(c.touches) != 1 {
		t.Fatal("read wrote DB or failed to coalesce")
	}
	if err := c.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	if err := c.Flush(t.Context()); err != nil || f.touches != 1 {
		t.Fatal("hourly buffer throttle", f.touches, err)
	}
	bad := s
	bad.Digest = strings.Repeat("f", 64)
	if _, err := c.Read(t.Context(), bad); err != ErrNotFound {
		t.Fatal("corruption leaked", err)
	}
	if err := os.Remove(filepath.Join(root, s.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(t.Context(), s); err != ErrNotFound {
		t.Fatal("missing file became 500", err)
	}
	// Sampling is bounded, and clean stale entries cannot permanently fill it.
	c.mu.Lock()
	c.touches = map[int64]touch{}
	c.mu.Unlock()
	for i := 0; i < maxTouches+10; i++ {
		s.ItemID = int64(100000 + i)
		c.record(s)
	}
	if len(c.touches) != maxTouches {
		t.Fatal("unbounded memory")
	}
	c.mu.Lock()
	for id, v := range c.touches {
		v.dirty = false
		v.seen = time.Now().Add(-2 * time.Hour)
		c.touches[id] = v
	}
	c.mu.Unlock()
	if err := c.Flush(t.Context()); err != nil || len(c.touches) != 0 {
		t.Fatal("sampling capacity stuck", err)
	}
}
func TestCacheBoundedGCResumesAndNeverEscapes(t *testing.T) {
	f := newFakeCache()
	c, root := testManagedCache(t, f)
	live := snapshotImage(t, c, 100001, testAsset)
	f.snapshots[live.ItemID] = live
	ageFile(t, root, live.RelativePath)
	orphan := snapshotImage(t, c, 100002, "cd222222-2222-4222-8222-222222222222")
	ageFile(t, root, orphan.RelativePath)
	fresh := snapshotImage(t, c, 100003, "ef333333-3333-4333-8333-333333333333")
	active := snapshotImage(t, c, 100004, "bb444444-4444-4444-8444-444444444444")
	ageFile(t, root, active.RelativePath)
	f.active[active.RelativePath] = true
	tmp := "ab/.tmp-" + strings.Repeat("c", 32)
	if err := os.WriteFile(filepath.Join(root, tmp), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	ageFile(t, root, tmp)
	keep := filepath.Join(root, "ab", "keep.txt")
	if err := os.WriteFile(keep, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := realTemp(t)
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "fe")); err != nil {
		t.Fatal(err)
	}
	drainGC(t, c, 3)
	for _, p := range []string{orphan.RelativePath, tmp} {
		if _, err := os.Stat(filepath.Join(root, p)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("old orphan survived", p, err)
		}
	}
	for _, p := range []string{live.RelativePath, fresh.RelativePath, active.RelativePath, ".lock", "ab/keep.txt"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Fatal("unsafe collection", p, err)
		}
	}
	raw, err := os.ReadFile(sentinel)
	if err != nil || !bytes.Equal(raw, []byte("outside")) {
		t.Fatal("escaped still root", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GC(t.Context(), 1); err != ErrUnavailable {
		t.Fatal("closed GC", err)
	}
}
func TestCacheGCCancellationRetainsPendingEntry(t *testing.T) {
	f := newFakeCache()
	c, root := testManagedCache(t, f)
	orphan := snapshotImage(t, c, 100001, testAsset)
	ageFile(t, root, orphan.RelativePath)
	entered := make(chan struct{})
	f.referenceHook = func(ctx context.Context, _ string) (bool, error) {
		close(entered)
		<-ctx.Done()
		return false, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.GC(ctx, 200); done <- err }()
	select {
	case <-entered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("GC did not reach pending entry")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.cursor.pending != orphan.RelativePath {
		t.Fatal("lost continuation")
	}
	f.referenceHook = nil
	drainGC(t, c, 200)
	if _, err := os.Stat(filepath.Join(root, orphan.RelativePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("resume skipped entry", err)
	}
}
func TestCacheSizeCorrectionAndConstructorBounds(t *testing.T) {
	f := newFakeCache()
	c, _ := testManagedCache(t, f)
	s := snapshotImage(t, c, 100001, testAsset)
	s.SizeBytes = 1
	f.snapshots[s.ItemID] = s
	if r, err := c.GC(t.Context(), 1); err != nil || r.Visited != 1 {
		t.Fatal(r, err)
	}
	if f.snapshots[s.ItemID].SizeBytes <= 1 {
		t.Fatal("size drift not corrected")
	}
	if _, err := c.GC(t.Context(), 201); err != ErrInvalid {
		t.Fatal(err)
	}
	f.cfg.GCGrace = 2 * stillqueue.MaxLease
	if _, err := newCache(c.store, f); err != ErrInvalid {
		t.Fatal("unsafe GC grace")
	}
	if _, err := NewCache(c.store, nil); err != ErrInvalid {
		t.Fatal("nil repository")
	}
}

func FuzzStoredPathWhitelist(f *testing.F) {
	f.Add("ab/.tmp-" + strings.Repeat("c", 32))
	f.Add("ab/" + testAsset + "-" + testFP[:16] + ".jpg")
	f.Add("../../outside")
	f.Add("")
	f.Fuzz(func(t *testing.T, path string) {
		if validStoredPath(path) && (strings.Count(path, "/") != 1 || path[2] != '/' || strings.ContainsAny(path, "\\\x00") || strings.Contains(path, "..")) {
			t.Fatal("unsafe GC path")
		}
	})
}
