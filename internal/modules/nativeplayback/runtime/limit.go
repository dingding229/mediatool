package runtime

import (
	"context"
	"errors"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/storage"
	"sync/atomic"
	"time"
)

// Match the gateway cache's four source lanes, including when Redis is absent.
// This is BELOW the UA/IP/version cache, so a 115 cooldown never blocks valid
// cache hits. No DB/user lock is held while waiting or doing provider I/O.
type limitedSource struct {
	source storage.PlaybackResolver
	lanes  chan struct{}
	after  atomic.Int64
}

func (l *limitedSource) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	wait := func() error {
		if d := time.Until(time.Unix(0, l.after.Load())); d > 0 {
			return &nativeplayback.RetryError{After: d}
		}
		return nil
	}
	if err := wait(); err != nil {
		return storage.DownloadLink{}, err
	}
	select {
	case l.lanes <- struct{}{}:
	case <-ctx.Done():
		return storage.DownloadLink{}, ctx.Err()
	}
	defer func() { <-l.lanes }()
	if err := wait(); err != nil {
		return storage.DownloadLink{}, err
	}
	link, err := l.source.Resolve(ctx, ref)
	var api *open115.APIError
	if errors.Is(err, open115.ErrRateLimited) || errors.As(err, &api) && api.RateLimited() {
		l.after.Store(time.Now().Add(time.Minute).UnixNano())
		return storage.DownloadLink{}, &nativeplayback.RetryError{After: time.Minute}
	}
	return link, err
}
