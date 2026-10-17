package stillstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const gcBudget = 2 * time.Second

type cursor struct {
	after   int64
	files   bool
	shard   int
	dir     *os.File
	pending string
}
type GCReport struct {
	Visited        int
	Reclaimed      int
	ReclaimedBytes int64
	Failures       uint64
	Done           bool
}

// GC retains an open directory cursor between bounded calls. A process restart
// safely starts a new cycle; it never trusts a client path or follows symlinks.
func (c *Cache) GC(parent context.Context, limit int) (report GCReport, err error) {
	if limit < 1 || limit > 200 {
		return report, ErrInvalid
	}
	if !c.gcMu.TryLock() {
		return report, ErrUnavailable
	}
	defer c.gcMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, gcBudget)
	defer cancel()
	err = c.withLock(ctx, func() error {
		before := c.failures.Load()
		defer func() { report.Failures = c.failures.Load() - before }()
		rows, e := c.reconcile(ctx, 0, true, limit, &report)
		report.Visited += len(rows)
		if e != nil {
			return e
		}
		if report.Visited == limit {
			return nil
		}
		// Calibrated historical sizes can exceed the quota: trim by persisted LRU.
		u, e := c.repo.Usage(ctx)
		if e != nil {
			return e
		}
		used := u.SnapshotBytes + u.ReservedBytes
		if used > c.limits.MaxBytes {
			oldest, e := c.repo.Oldest(ctx, limit-report.Visited)
			if e != nil {
				return e
			}
			for _, s := range oldest {
				if used <= c.limits.MaxBytes*9/10 {
					break
				}
				if e = ctx.Err(); e != nil {
					return e
				}
				ok, e := c.repo.EvictSnapshot(ctx, s)
				if e != nil {
					return e
				}
				report.Visited++
				if ok {
					used -= s.SizeBytes
					if removed, size := c.cleanup(ctx, s.RelativePath, false); removed {
						report.Reclaimed++
						report.ReclaimedBytes += size
					}
				}
			}
		}
		if report.Visited < limit && !c.cursor.files {
			rows, e := c.reconcile(ctx, c.cursor.after, false, limit-report.Visited, &report)
			for _, s := range rows {
				c.cursor.after = s.ItemID
			}
			report.Visited += len(rows)
			if e != nil {
				return e
			}
			if report.Visited < limit {
				c.cursor.files = true
			}
		}
		for report.Visited < limit && c.cursor.files {
			if e = ctx.Err(); e != nil {
				return e
			}
			if c.cursor.shard == 256 {
				c.cursor.files = false
				c.cursor.shard = 0
				c.cursor.after = 0
				report.Done = true
				break
			}
			path, e := c.next()
			if e != nil {
				return e
			}
			report.Visited++
			if path != "" {
				if removed, size := c.cleanup(ctx, path, true); removed {
					report.Reclaimed++
					report.ReclaimedBytes += size
				}
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			c.cursor.pending = ""
		}
		return nil
	})
	return report, err
}
func (c *Cache) next() (path string, err error) {
	if c.cursor.pending != "" {
		return c.cursor.pending, nil
	}
	shard := fmt.Sprintf("%02x", c.cursor.shard)
	if c.cursor.dir == nil {
		dir, e := openShard(c.store.root, shard, false)
		if e != nil {
			if !errors.Is(e, ErrNotFound) {
				c.failures.Add(1)
			}
			c.cursor.shard++
			return "", nil
		} // Missing/unsafe shards never get followed.
		c.cursor.dir = dir
	}
	entries, e := c.cursor.dir.ReadDir(1)
	if len(entries) > 0 {
		c.cursor.pending = shard + "/" + entries[0].Name()
	}
	if e != nil && !errors.Is(e, io.EOF) {
		err = ErrUnavailable
		closeFile(c.cursor.dir, &err)
		c.cursor.dir = nil
		c.cursor.pending = ""
		c.cursor.shard++
		return "", err
	}
	if len(entries) == 0 {
		closeFile(c.cursor.dir, &err)
		c.cursor.dir = nil
		c.cursor.shard++
		return "", err
	}
	c.cursor.pending = shard + "/" + entries[0].Name()
	return c.cursor.pending, nil
}
func validStoredPath(path string) bool {
	if _, _, err := splitPath(path); err == nil {
		return true
	}
	return len(path) == 40 && path[2] == '/' && path[3:8] == ".tmp-" && strings.Trim(path[:2]+path[8:], "0123456789abcdef") == ""
}
func (c *Cache) measure(path string) (size int64, err error) {
	if _, _, err = splitPath(path); err != nil {
		return 0, err
	}
	file, dir, err := c.openFile(path)
	if err != nil {
		return 0, err
	}
	defer closeFile(dir, &err)
	defer closeFile(file, &err)
	st, err := file.Stat()
	if err != nil || st.Size() < 1 || st.Size() > int64(MaxJPEGBytes) {
		return 0, ErrInvalid
	}
	return st.Size(), nil
}
func (c *Cache) openFile(path string) (*os.File, *os.File, error) {
	if !validStoredPath(path) {
		return nil, nil, ErrUnsafe
	}
	dir, err := openShard(c.store.root, path[:2], false)
	if err != nil {
		return nil, nil, err
	}
	file, err := openLeaf(dir, path[3:])
	if err != nil {
		closeFile(dir, &err)
		return nil, nil, err
	}
	return file, dir, nil
}

// Deletion failures are counted, not returned to publishers: the committed
// reference is already gone, and its orphan must not block unrelated production.
func (c *Cache) cleanup(ctx context.Context, path string, grace bool) (bool, int64) {
	size, err := c.remove(ctx, path, grace)
	if err != nil && !errors.Is(err, ErrNotFound) {
		c.failures.Add(1)
	}
	return err == nil && size >= 0, max(0, size)
}
func (c *Cache) remove(ctx context.Context, path string, grace bool) (size int64, err error) {
	if !validStoredPath(path) {
		return -1, nil
	} // Never touch .lock or unrelated filenames.
	if err = ctx.Err(); err != nil {
		return -1, err
	}
	file, dir, err := c.openFile(path)
	if err != nil {
		return -1, err
	}
	defer closeFile(dir, &err)
	defer closeFile(file, &err)
	st, err := file.Stat()
	if err != nil {
		return -1, ErrUnavailable
	}
	if grace && time.Since(st.ModTime()) < c.limits.GCGrace {
		return -1, nil
	}
	if !strings.HasPrefix(path[3:], ".tmp-") {
		used, e := c.repo.Referenced(ctx, path)
		if e != nil {
			return -1, e
		}
		if used {
			return -1, nil
		}
	}
	if err = ctx.Err(); err != nil {
		return -1, err
	}
	discard(dir, path[3:], &err)
	return st.Size(), err
}
