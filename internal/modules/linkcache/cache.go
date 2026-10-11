package linkcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

type Cached struct {
	Source         storage.PlaybackResolver
	Redis          *redis.Client
	Version        func(context.Context, storage.FileRef) (string, error)
	ResolveTimeout time.Duration
	flight         singleflight.Group
	gate           chan struct{}
	once           sync.Once
}

func (c *Cached) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	if ctx.Err() != nil {
		return storage.DownloadLink{}, ctx.Err()
	}
	if c.Source == nil {
		return storage.DownloadLink{}, storage.ErrUnavailable
	}
	if c.Redis == nil {
		return c.Source.Resolve(ctx, ref)
	}
	version := ""
	if c.Version != nil {
		started := time.Now()
		v, err := c.Version(ctx, ref)
		err = storage.ContextFailure(ctx, err)
		storage.ObserveResolve(ctx, storage.StageCacheVersion, started, err)
		if err != nil {
			return storage.DownloadLink{}, err
		}
		version = v
	}
	raw, _ := json.Marshal(struct {
		Ref     storage.FileRef
		Version string
		UA, IP  string
	}{ref, version, ref.UserAgent, ref.ClientIP})
	digest := sha256.Sum256(raw)
	key := "media-tool:download-link:" + hex.EncodeToString(digest[:])
	lookedUp := time.Now()
	lookup, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	cached, err := c.Redis.Get(lookup, key).Bytes()
	cancel()
	observedErr := err
	if errors.Is(err, redis.Nil) {
		observedErr = nil
	}
	storage.ObserveResolve(ctx, storage.StageCacheLookup, lookedUp, observedErr)
	if err == nil {
		var link storage.DownloadLink
		if json.Unmarshal(cached, &link) == nil && cacheTTL(link) > 0 {
			storage.ObserveResolve(ctx, storage.StageCacheHit, time.Now(), nil)
			return link, nil
		}
	}
	if ctx.Err() != nil {
		return storage.DownloadLink{}, ctx.Err()
	}
	storage.ObserveResolve(ctx, storage.StageCacheMiss, time.Now(), nil)
	waited := time.Now()
	result := c.flight.DoChan(key, func() (any, error) {
		c.once.Do(func() { c.gate = make(chan struct{}, 4) })
		sourceCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), requestbudget.EffectiveResolve(c.ResolveTimeout))
		defer stop()
		queued := time.Now()
		select {
		case c.gate <- struct{}{}:
		case <-sourceCtx.Done():
			storage.ObserveResolve(sourceCtx, storage.StageSourceQueue, queued, sourceCtx.Err())
			return storage.DownloadLink{}, sourceCtx.Err()
		}
		if sourceCtx.Err() != nil {
			storage.ObserveResolve(sourceCtx, storage.StageSourceQueue, queued, sourceCtx.Err())
			<-c.gate
			return storage.DownloadLink{}, sourceCtx.Err()
		}
		storage.ObserveResolve(sourceCtx, storage.StageSourceQueue, queued, nil)
		defer func() { <-c.gate }()
		started := time.Now()
		link, err := c.Source.Resolve(sourceCtx, ref)
		err = storage.ContextFailure(sourceCtx, err)
		storage.ObserveResolve(sourceCtx, storage.StageSourceResolve, started, err)
		if err != nil {
			return link, err
		}
		ttl := cacheTTL(link)
		if ttl <= 0 {
			return link, nil
		}
		ttl = min(ttl, 2*time.Minute)
		value, _ := json.Marshal(link)
		save, done := context.WithTimeout(sourceCtx, 250*time.Millisecond)
		defer done()
		c.Redis.Set(save, key, value, ttl)
		return link, nil
	})
	storage.ObserveResolve(ctx, storage.StageCacheJoined, time.Now(), nil)
	select {
	case <-ctx.Done():
		storage.ObserveResolve(ctx, storage.StageCacheWait, waited, ctx.Err())
		return storage.DownloadLink{}, ctx.Err()
	case r := <-result:
		storage.ObserveResolve(ctx, storage.StageCacheWait, waited, r.Err)
		if r.Err != nil {
			return storage.DownloadLink{}, r.Err
		}
		return r.Val.(storage.DownloadLink), nil
	}
}
func (c *Cached) Invalidate(ctx context.Context, ref storage.FileRef) {
	if c.Redis == nil {
		return
	}
	version := ""
	if c.Version != nil {
		version, _ = c.Version(ctx, ref)
	}
	raw, _ := json.Marshal(struct {
		Ref     storage.FileRef
		Version string
		UA, IP  string
	}{ref, version, ref.UserAgent, ref.ClientIP})
	hash := sha256.Sum256(raw)
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	c.Redis.Del(bounded, "media-tool:download-link:"+hex.EncodeToString(hash[:]))
}

// CacheUntil is the program's short cache bound, not a claim about provider expiry.
func cacheTTL(link storage.DownloadLink) time.Duration {
	limit := time.Duration(0)
	if !link.CacheUntil.IsZero() {
		limit = time.Until(link.CacheUntil) - 5*time.Second
	}
	if !link.ExpiresAt.IsZero() {
		provider := time.Until(link.ExpiresAt) - 30*time.Second
		if limit == 0 || provider < limit {
			limit = provider
		}
	}
	return min(limit, 2*time.Minute)
}
