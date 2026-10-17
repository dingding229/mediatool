package stillstore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"media-tool/internal/modules/mediaprobe/stillqueue"
)

var ErrRetry = errors.New("still cache retry")

type RetryError struct {
	Reason stillqueue.AttemptReason
	After  time.Duration
}

func (*RetryError) Error() string { return "still cache retry" }
func (*RetryError) Unwrap() error { return ErrRetry }

// No timers/workers start here. C owns Flush/GC scheduling and fenced Succeed.
type cacheBackend interface {
	Limits() stillqueue.Config
	Admit(context.Context, stillqueue.Lease, int64) (stillqueue.Admission, error)
	SnapshotPage(context.Context, int64, bool, int) ([]stillqueue.Snapshot, error)
	Oldest(context.Context, int) ([]stillqueue.Snapshot, error)
	Calibrate(context.Context, stillqueue.Snapshot, int64) (bool, error)
	EvictSnapshot(context.Context, stillqueue.Snapshot) (bool, error)
	Referenced(context.Context, string) (bool, error)
	TouchBatch(context.Context, []stillqueue.Access) (int64, error)
	Usage(context.Context) (stillqueue.Usage, error)
}
type Cache struct {
	store    *Store
	repo     cacheBackend
	limits   stillqueue.Config
	mu       sync.Mutex
	touches  map[int64]touch
	gcMu     sync.Mutex
	cursor   cursor
	failures atomic.Uint64
}

func NewCache(s *Store, r *stillqueue.Repository) (*Cache, error) {
	if r == nil {
		return nil, ErrInvalid
	}
	return newCache(s, r)
}
func newCache(s *Store, r cacheBackend) (*Cache, error) {
	if s == nil || r == nil {
		return nil, ErrInvalid
	}
	limits, err := r.Limits().Normalize()
	if err != nil {
		return nil, ErrInvalid
	}
	return &Cache{store: s, repo: r, limits: limits, touches: make(map[int64]touch)}, nil
}
func (c *Cache) Close() error {
	c.gcMu.Lock()
	defer c.gcMu.Unlock()
	var err error
	if c.cursor.dir != nil {
		closeFile(c.cursor.dir, &err)
		c.cursor.dir = nil
	}
	return errors.Join(err, c.store.Close())
}
func (c *Cache) CleanupFailures() uint64 { return c.failures.Load() }

// Publish holds the same cross-process lock as raw B1 writes, covering admission
// and post-commit unlink. A receipt still requires C's fenced Succeed afterwards.
func (c *Cache) Publish(ctx context.Context, l stillqueue.Lease, raw []byte) (stillqueue.Image, error) {
	image, err := c.store.publish(ctx, l.AssetID, l.Fingerprint, raw, func(ctx context.Context, wanted stillqueue.Image) error {
		if _, err := c.reconcile(ctx, 0, true, stillqueue.MaxCacheBatch); err != nil {
			return err
		}
		if err := c.Flush(ctx); err != nil {
			return err
		}
		a, err := c.repo.Admit(ctx, l, wanted.SizeBytes)
		if err != nil {
			return err
		}
		for _, s := range a.Evicted {
			c.cleanup(ctx, s.RelativePath, false)
		}
		if !a.Reserved {
			return &RetryError{stillqueue.StorageFull, time.Minute}
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrConflict) && !errors.Is(err, stillqueue.ErrLeaseLost) && !errors.Is(err, ErrRetry) {
		err = &RetryError{stillqueue.StorageUnavailable, time.Minute}
	}
	return image, err
}

// Read returns not-found for missing/corrupt cached files; D falls back to TMDB.
// Only a matching receipt records access; no database call is made on this path.
func (c *Cache) Read(ctx context.Context, s stillqueue.Snapshot) ([]byte, error) {
	raw, image, err := c.store.Read(ctx, s.RelativePath)
	if err != nil || image.Digest != s.Digest {
		return nil, ErrNotFound
	}
	c.record(s)
	return raw, nil
}
func (c *Cache) withLock(ctx context.Context, fn func() error) (err error) {
	c.store.mu.RLock()
	defer c.store.mu.RUnlock()
	if c.store.root == nil {
		return ErrUnavailable
	}
	lock, err := lockRoot(ctx, c.store.root)
	if err != nil {
		return err
	}
	defer closeFile(lock, &err)
	return fn()
}

// Unknown rows are consumed from the first page: correcting/dropping each row
// removes it from that set. A failed stat cannot become a global sticky latch.
func (c *Cache) reconcile(ctx context.Context, after int64, unknown bool, limit int, reports ...*GCReport) ([]stillqueue.Snapshot, error) {
	rows, err := c.repo.SnapshotPage(ctx, after, unknown, limit)
	if err != nil {
		return nil, err
	}
	for i, s := range rows {
		if err = ctx.Err(); err != nil {
			return rows[:i], err
		}
		size, statErr := c.measure(s.RelativePath)
		if statErr != nil {
			size = 0
		}
		if statErr == nil && size == s.SizeBytes {
			continue
		}
		ok, e := c.repo.Calibrate(ctx, s, size)
		if e != nil {
			return rows[:i], e
		}
		if ok && size == 0 {
			if removed, bytes := c.cleanup(ctx, s.RelativePath, false); removed && len(reports) > 0 {
				reports[0].Reclaimed++
				reports[0].ReclaimedBytes += bytes
			}
		}
	}
	return rows, nil
}
