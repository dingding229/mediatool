package stillstore

import (
	"context"
	"time"

	"media-tool/internal/modules/mediaprobe/stillqueue"
)

const maxTouches = 4096

type touch struct {
	access     stillqueue.Access
	seen, next time.Time
	dirty      bool
}

func (c *Cache) record(s stillqueue.Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.touches[s.ItemID]
	if !ok && len(c.touches) >= maxTouches {
		return
	} // Bounded best-effort access sampling.
	if !ok || t.access.RelativePath != s.RelativePath || t.access.Digest != s.Digest {
		t = touch{access: stillqueue.Access{ItemID: s.ItemID, RelativePath: s.RelativePath, Digest: s.Digest}, next: s.LastAccessAt.Add(time.Hour)}
	}
	t.seen = time.Now()
	t.dirty = true
	c.touches[s.ItemID] = t
}
func (c *Cache) Flush(ctx context.Context) error {
	c.mu.Lock()
	batch := make([]stillqueue.Access, 0, stillqueue.MaxCacheBatch)
	sampled := make(map[int64]touch)
	for id, t := range c.touches {
		if !t.dirty && time.Since(t.seen) > time.Hour {
			delete(c.touches, id)
			continue
		}
		if t.dirty && !time.Now().Before(t.next) {
			sampled[id] = t
			batch = append(batch, t.access)
			if len(batch) == stillqueue.MaxCacheBatch {
				break
			}
		}
	}
	c.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	if _, err := c.repo.TouchBatch(ctx, batch); err != nil {
		return err
	} // Keep dirty entries on failure.
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, old := range sampled {
		t := c.touches[id]
		if t.access != old.access {
			continue
		}
		t.next = time.Now().Add(time.Hour)
		t.dirty = !t.seen.Equal(old.seen)
		c.touches[id] = t
	}
	return nil
}
