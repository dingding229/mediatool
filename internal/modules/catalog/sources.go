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
type SourceRegistry struct {
	Store pluginv2.Store
	Cache *redis.Client
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
type sourceCache struct {
	Snapshot string                 `json:"snapshot"`
	Result   contracts.SourceResult `json:"result"`
}

func sourceKey(id, kind string) string { return "plugins:v2:sources:" + id + ":" + kind }
func (s SourceRegistry) Refresh(ctx context.Context) error {
	if s.Cache == nil {
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
			q := contracts.SourceRequest{Page: 1, Size: 200}
			result, e := s.Store.SourcePage(bounded, p, kind, q)
			if e != nil {
				result = contracts.SourceResult{Items: []contracts.SourceCandidate{}}
			}
			raw, _ := json.Marshal(sourceCache{Snapshot: p.ID, Result: result})
			if e = s.Cache.Set(bounded, sourceKey(p.InstanceID, string(kind)), raw, 2*time.Minute).Err(); e != nil {
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
	if s.Cache == nil {
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
		raw, e := s.Cache.Get(ctx, sourceKey(p.InstanceID, kind)).Bytes()
		if e != nil {
			continue
		}
		var v sourceCache
		if len(raw) > 256<<10 || contracts.Decode(raw, &v) != nil || v.Snapshot != p.ID || contracts.ValidateSource(contracts.SourceRequest{Page: 1, Size: 200}, v.Result) != nil {
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
