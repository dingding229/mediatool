package search

import (
	"context"
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/metadata"
	"time"
)

func (e Engine) StartMediaCached(ctx context.Context, keyword string, ids []string, refresh bool, media metadata.Item) (string, bool, error) {
	if media.ID < 1 || (media.MediaType != "movie" && media.MediaType != "series") || validateAccountIDs(ids) != nil {
		return "", false, ErrSnapshotChanged
	}
	key, ttl, err := e.cacheKey(ctx, keyword, ids)
	if err != nil {
		return "", false, err
	}
	key += fmt.Sprintf(":media:%s:%d", media.MediaType, media.ID)
	if !refresh && ttl > 0 && e.Redis != nil {
		id, err := e.Redis.Get(ctx, key).Result()
		if err == nil {
			run, err := e.Get(ctx, id, 1, 1)
			var saved metadata.Item
			json.Unmarshal(run.Media, &saved)
			if err == nil && saved.ID == media.ID && saved.MediaType == media.MediaType && (run.State == "succeeded" || run.State == "partial") && run.FinishedAt != nil && time.Since(*run.FinishedAt) < ttl {
				var visible bool
				if e.DB.QueryRow(ctx, `SELECT NOT cache_hidden FROM search_runs WHERE id=$1`, id).Scan(&visible) == nil && visible {
					return id, true, nil
				}
			}
		}
	}
	id, err := e.start(ctx, keyword, ids, true, key, media)
	return id, false, err
}
