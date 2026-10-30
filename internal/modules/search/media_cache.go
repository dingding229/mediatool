package search

import (
	"context"
	"crypto/sha256"
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
	// A new identity/alias snapshot must never reuse an old run's frozen media.
	raw, err := json.Marshal(media.ShowMetadata())
	if err != nil {
		return "", false, err
	}
	key += fmt.Sprintf(":media:%s:%d:identity-v2:%x", media.MediaType, media.ID, sha256.Sum256(raw))
	if !refresh && ttl > 0 && e.Redis != nil {
		id, err := e.Redis.Get(ctx, key).Result()
		if err == nil {
			run, err := e.Get(ctx, id, 1, 1)
			var saved metadata.Item
			// A missing/invalid cache identity is a miss, never a valid saved search.
			if err == nil {
				if decodeErr := json.Unmarshal(run.Media, &saved); decodeErr != nil {
					err = decodeErr
				}
			}
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
