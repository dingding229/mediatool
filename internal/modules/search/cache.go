package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/sites"
	"sort"
	"time"
)

func (e Engine) cacheKey(ctx context.Context, keyword string, ids []string) (string, time.Duration, error) {
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return "", 0, err
	}
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	profiles := []any{}
	for _, id := range sorted {
		c, err := e.Connections.Repository.Get(ctx, id)
		if err != nil {
			return "", 0, err
		}
		if c.Record.Kind != "ptsite" || !c.Record.Enabled {
			return "", 0, ErrSnapshotChanged
		}
		def, err := (sites.Repository{DB: e.DB}).Get(ctx, c.String("definitionId"))
		if err != nil {
			return "", 0, err
		}
		profiles = append(profiles, []any{id, c.Record.Revision, sites.DefinitionFingerprint(def.RawJSON, def.CommonJSON, def.SourceCommit)})
	}
	raw, _ := json.Marshal([]any{keyword, profiles, prefs.Revision})
	sum := sha256.Sum256(raw)
	return "media-tool:search-cache:" + hex.EncodeToString(sum[:]), time.Duration(prefs.Value.SearchCacheSeconds) * time.Second, nil
}
func (e Engine) StartCached(ctx context.Context, keyword string, ids []string, refresh bool) (string, bool, error) {
	// Start's validation still applies even on cache hits.
	if keyword == "" || len(keyword) > 200 || len(ids) == 0 || len(ids) > 10 {
		return "", false, ErrSnapshotChanged
	}
	key, ttl, err := e.cacheKey(ctx, keyword, ids)
	if err != nil {
		return "", false, err
	}
	if !refresh && ttl > 0 && e.Redis != nil {
		bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		id, err := e.Redis.Get(bounded, key).Result()
		cancel()
		if err == nil {
			run, err := e.Get(ctx, id, 1, 50)
			if err == nil && (run.State == "succeeded" || run.State == "partial") && run.FinishedAt != nil && time.Since(*run.FinishedAt) < ttl {
				return id, true, nil
			}
		}
	}
	id, err := e.start(ctx, keyword, ids, true, key)
	return id, false, err
}
func (e Engine) rememberCache(ctx context.Context, id string) {
	if e.Redis == nil {
		return
	}
	var key string
	err := e.DB.QueryRow(ctx, `SELECT coalesce(cache_key,'') FROM search_runs WHERE id=$1 AND state IN('succeeded','partial') AND NOT EXISTS(SELECT 1 FROM search_site_results WHERE run_id=$1 AND state NOT IN('succeeded','partial'))`, id).Scan(&key)
	if err != nil || key == "" {
		return
	}
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil || prefs.Value.SearchCacheSeconds == 0 {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	e.Redis.Set(bounded, key, id, time.Duration(prefs.Value.SearchCacheSeconds)*time.Second)
}
