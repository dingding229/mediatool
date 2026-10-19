package catalog

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/redis/go-redis/v9"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"time"
)

// SourceRegistry is an additional, attributed provider cache, not a fictitious
// built-in recommender. Only Refresh may call workers, from the background loop.
type SourceBackend interface {
	SourceProviders(context.Context, contracts.ChainEventType) ([]pluginv2.Snapshot, error)
	SourcePage(context.Context, pluginv2.Snapshot, contracts.ChainEventType, contracts.SourceRequest) (contracts.SourceResult, error)
	Descriptor(context.Context, string, string, string) (pluginv2.Snapshot, contracts.Descriptor, error)
}
type SourceRegistry struct {
	Store SourceBackend
	Cache *redis.Client
	Now   func() time.Time // Optional clock; runtime always uses the local wall clock.
}

func (s SourceRegistry) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type SourceCard struct {
	contracts.SourceCandidate
	Source     string `json:"source"`
	InstanceID string `json:"instanceId"`
	Kind       string `json:"kind"`
}
type SourcePage struct {
	Items     []SourceCard `json:"items"`
	Page      int          `json:"page"`
	Size      int          `json:"size"`
	HasMore   bool         `json:"hasMore"`
	Truncated bool         `json:"truncated"`
}

const sourceFresh = 2 * time.Minute
const sourceStateTTL = time.Hour
const sourceCacheVersion = 2

type sourceCache struct {
	Version     int                    `json:"version"`
	Snapshot    string                 `json:"snapshot"`
	Result      contracts.SourceResult `json:"result"`
	WrittenAt   time.Time              `json:"writtenAt"`
	FreshUntil  time.Time              `json:"freshUntil"`
	NextAt      time.Time              `json:"nextAt"`
	EmptyStreak int                    `json:"emptyStreak"`
	Failures    int                    `json:"failures"`
}

func sourceKey(id, kind string) string { return "plugins:v2:sources:" + id + ":" + kind }
func backoff(base, cap time.Duration, n int) time.Duration {
	return min(cap, base*time.Duration(1<<min(max(n-1, 0), 5)))
}
func (s SourceRegistry) cached(ctx context.Context, id, kind, snapshot string) (sourceCache, bool) {
	var v sourceCache
	raw, e := s.Cache.Get(ctx, sourceKey(id, kind)).Bytes()
	if e != nil || len(raw) > 256<<10 || contracts.Decode(raw, &v) != nil || v.Version != sourceCacheVersion || v.Snapshot != snapshot || v.WrittenAt.IsZero() || v.NextAt.Before(v.WrittenAt) || v.NextAt.Sub(v.WrittenAt) > 15*time.Minute || v.EmptyStreak < 0 || v.EmptyStreak > 6 || v.Failures < 0 || v.Failures > 6 || contracts.ValidateSource(contracts.SourceRequest{Page: 1, Size: 200}, v.Result) != nil {
		return sourceCache{}, false
	}
	if !v.FreshUntil.IsZero() && (v.FreshUntil.Before(v.WrittenAt) || v.FreshUntil.After(v.NextAt)) {
		return sourceCache{}, false
	}
	return v, true
}
func (s SourceRegistry) Refresh(ctx context.Context) error {
	if s.Cache == nil || s.Store == nil {
		return nil
	}
	// Coalesce across app workers. Lease expires even after process death.
	token := time.Now().UTC().Format(time.RFC3339Nano)
	key := "plugins:v2:sources:lease"
	ok, e := s.Cache.SetNX(ctx, key, token, 5*time.Second).Result()
	if e != nil || !ok {
		return e
	}
	// Lease deletion is best-effort: TTL releases it after a cache outage.
	defer func() {
		_ = s.Cache.Eval(context.WithoutCancel(ctx), `if redis.call('get',KEYS[1])==ARGV[1] then return redis.call('del',KEYS[1]) end return 0`, []string{key}, token).Err()
	}()
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, kind := range []contracts.ChainEventType{contracts.ChainDiscoverSource, contracts.ChainRecommendSource} {
		providers, e := s.Store.SourceProviders(bounded, kind)
		if e != nil {
			return e
		}
		for _, p := range providers {
			previous, ok := s.cached(bounded, p.InstanceID, string(kind), p.ID)
			if ok && s.now().Before(previous.NextAt) {
				continue
			}
			q := contracts.SourceRequest{Page: 1, Size: 200}
			result, e := s.Store.SourcePage(bounded, p, kind, q)
			now := s.now()
			entry := sourceCache{Version: sourceCacheVersion, Snapshot: p.ID, Result: result, WrittenAt: now, NextAt: now.Add(sourceFresh), FreshUntil: now.Add(sourceFresh)}
			if e != nil || contracts.ValidateSource(q, result) != nil {
				entry.Result = contracts.SourceResult{Items: []contracts.SourceCandidate{}}
				entry.Failures = min(previous.Failures+1, 6)
				entry.FreshUntil = time.Time{}
				entry.NextAt = now.Add(backoff(30*time.Second, 5*time.Minute, entry.Failures))
			} else if len(result.Items) == 0 {
				entry.EmptyStreak = min(previous.EmptyStreak+1, 6)
				entry.NextAt = now.Add(backoff(sourceFresh, 15*time.Minute, entry.EmptyStreak))
				entry.FreshUntil = entry.NextAt
			}
			// Retain bounded retry metadata beyond data freshness. Page enforces
			// FreshUntil independently, so failures never prolong stale cards.
			raw, _ := json.Marshal(entry)
			if e = s.Cache.Set(bounded, sourceKey(p.InstanceID, string(kind)), raw, sourceStateTTL).Err(); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s SourceRegistry) Page(ctx context.Context, kind string, page, size int) (SourcePage, error) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	out := SourcePage{Items: []SourceCard{}, Page: page, Size: size}
	if kind != "discover.source" && kind != "recommend.source" || page < 1 || page > 1000 || size < 1 || size > 200 {
		return out, ErrInvalid
	}
	if s.Cache == nil || s.Store == nil {
		return out, nil
	}
	// This path only reads current metadata and Redis. No plugin.invoke, mailbox
	// wait, network scrape, TMDB request or asynchronous cache fill is permitted.
	providers, e := s.Store.SourceProviders(ctx, contracts.ChainEventType(kind))
	if e != nil {
		return out, e
	}
	all := []SourceCard{}
	for _, p := range providers {
		v, ok := s.cached(ctx, p.InstanceID, kind, p.ID)
		if !ok || !s.now().Before(v.FreshUntil) {
			continue
		}
		// Read authorization again after cache access to close a disable/revoke race.
		latest, _, e := s.Store.Descriptor(ctx, p.InstanceID, "chain", kind)
		if e != nil || latest.ID != p.ID {
			continue
		}
		for _, c := range v.Result.Items {
			all = append(all, SourceCard{SourceCandidate: c, Source: "plugin:" + p.InstanceID, InstanceID: p.InstanceID, Kind: kind})
		}
		out.Truncated = out.Truncated || v.Result.HasMore
	}
	start := min((page-1)*size, len(all))
	end := min(start+size, len(all))
	out.Items = all[start:end]
	out.HasMore = end < len(all)
	return out, nil
}
