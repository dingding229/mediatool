package playback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"media-tool/internal/modules/storage"
)

var partialRange = regexp.MustCompile(`^bytes 0-([0-9]+)/([0-9]+)$`)

const directVerificationLifetime = 20 * time.Second
const maxDirectVerifications = 128

// Only digests and deadlines are retained: no signed URL, user credentials,
// negative result, media body or authorization decision is cached here.
// Resolver/grant/Emby checks and URL/DNS policy still run on every request.
type directCheckCache struct {
	mu      sync.Mutex
	entries map[[32]byte]time.Time
	flight  singleflight.Group
	slots   chan struct{}
}

func directVerificationKey(link storage.DownloadLink, agent, clientIP string) [32]byte {
	raw, _ := json.Marshal([]string{link.URL, agent, clientIP})
	return sha256.Sum256(raw)
}

func (c *directCheckCache) contains(key [32]byte, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline, ok := c.entries[key]
	if ok && !now.Before(deadline) {
		delete(c.entries, key)
		return false
	}
	return ok
}

func (c *directCheckCache) remember(key [32]byte, deadline, now time.Time) {
	if !deadline.After(now) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[[32]byte]time.Time)
	}
	for k, expiry := range c.entries {
		if !now.Before(expiry) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) >= maxDirectVerifications {
		var oldest [32]byte
		var earliest time.Time
		for k, expiry := range c.entries {
			if earliest.IsZero() || expiry.Before(earliest) {
				oldest, earliest = k, expiry
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = deadline
}

func directVerificationDeadline(link storage.DownloadLink, started time.Time) time.Time {
	// Unknown provider lifetime: coalesce an in-flight check, but don't remember it.
	if link.CacheUntil.IsZero() && link.ExpiresAt.IsZero() {
		return started
	}
	deadline := started.Add(directVerificationLifetime)
	for _, bound := range []time.Time{link.CacheUntil, link.ExpiresAt} {
		if !bound.IsZero() && bound.Add(-5*time.Second).Before(deadline) {
			deadline = bound.Add(-5 * time.Second)
		}
	}
	return deadline
}

// Official direct-only delivery checks at most 1 KiB with the exact player UA.
// FFmpeg's repeated Range reopen passes through the fixed STRM URL each time;
// re-probing the same signed link on each reopen unnecessarily serializes CDN
// round trips. A positive observation is reusable for at most twenty seconds,
// partitioned by the full signed URL, UA and client IP and bounded by link life.
func (e *Engine) checkDirect(ctx context.Context, link storage.DownloadLink, agent, clientIP string) (passed bool) {
	started := time.Now()
	defer func() {
		var err error
		if !passed {
			if ctx.Err() != nil {
				err = ctx.Err()
			} else {
				err = storage.ErrNoDirectURL
			}
		}
		storage.ObserveResolve(ctx, storage.StageCDNCheck, started, err)
	}()
	if ctx.Err() != nil {
		return false
	}
	key := directVerificationKey(link, agent, clientIP)
	deadline := directVerificationDeadline(link, time.Now())
	if deadline.After(time.Now()) && e.directChecks.contains(key, time.Now()) {
		return true
	}
	result := e.directChecks.flight.DoChan(hex.EncodeToString(key[:]), func() (any, error) {
		started := time.Now()
		deadline := directVerificationDeadline(link, started)
		if deadline.After(started) && e.directChecks.contains(key, started) {
			return true, nil
		}
		// One cancelled player must not abort a check already shared by other players.
		// Queueing plus network work has a single finite ten-second bound.
		check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		select {
		case e.directChecks.slots <- struct{}{}:
		case <-check.Done():
			return false, nil
		}
		defer func() { <-e.directChecks.slots }()
		verified := e.probeDirect(check, link, agent)
		if verified {
			e.directChecks.remember(key, deadline, time.Now())
		}
		return verified, nil
	})
	select {
	case <-ctx.Done():
		return false
	case r := <-result:
		return ctx.Err() == nil && r.Err == nil && r.Val == true
	}
}

func (e *Engine) probeDirect(ctx context.Context, link storage.DownloadLink, agent string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", link.URL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Range", "bytes=0-1023")
	res, err := e.client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != 206 {
		return false
	}
	match := partialRange.FindStringSubmatch(res.Header.Get("Content-Range"))
	if len(match) != 3 {
		return false
	}
	end, endErr := strconv.ParseInt(match[1], 10, 64)
	total, totalErr := strconv.ParseInt(match[2], 10, 64)
	if endErr != nil || totalErr != nil || end < 0 || end > 1023 || total <= end {
		return false
	}
	typ := ""
	if raw := res.Header.Get("Content-Type"); raw != "" {
		var err error
		typ, _, err = mime.ParseMediaType(raw)
		if err != nil {
			return false
		}
	}
	if typ != "" && typ != "application/octet-stream" && !strings.HasPrefix(typ, "video/") && !strings.HasPrefix(typ, "audio/") {
		return false
	}
	sample, err := io.ReadAll(io.LimitReader(res.Body, 1024))
	return err == nil && int64(len(sample)) == end+1
}
