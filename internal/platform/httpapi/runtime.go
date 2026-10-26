package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"

	"strings"
	"time"
)

func (a *API) builder(ctx context.Context) strm.Builder {
	b := a.Builder
	if origin, err := settings.PlaybackOrigin(ctx, a.Deps.DB, b.BaseURL); err == nil {
		b.BaseURL = origin
	}
	if a.Deps.Integrations != nil {
		if c, err := a.Deps.Integrations.Repository.Default(ctx, "clouddrive2"); err == nil {
			root := strings.TrimRight(c.String("cloudRoot"), "/")
			if root != "" {
				b.AllowedRoots = []string{root}
			}
		}
	}
	return b
}
func (a *API) storageConfigured(ctx context.Context) bool {
	if a.Deps.Integrations != nil {
		_, err := a.Deps.Integrations.Repository.Default(ctx, "clouddrive2")
		if err == nil {
			return true
		}
		if !errors.Is(err, integrations.ErrNotFound) {
			return false
		}
		var configured bool
		if a.Deps.Integrations.Repository.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE kind='clouddrive2')`).Scan(&configured) == nil && configured {
			return false
		}
		return a.Config.CloudDriveEndpoint != ""
	}
	return a.Deps.Storage != nil
}
func (a *API) workerStatus(ctx context.Context) map[string]any {
	if a.Deps.DB == nil {
		return map[string]any{"state": "unavailable", "count": 0}
	}
	var count int
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if a.Deps.DB.QueryRow(bounded, `SELECT count(*) FROM worker_heartbeats WHERE last_seen_at>now()-interval '20 seconds'`).Scan(&count) != nil {
		return map[string]any{"state": "unavailable", "count": 0}
	}
	state := "stopped"
	if count > 0 {
		state = "ready"
	}
	return map[string]any{"state": state, "count": count}
}

// SPA on a separate listener. Clone the URL before adding the internal mount.
func (a *API) ownedAsset(ctx context.Context, id string) bool {
	if a.Deps.DB == nil {
		return false
	}
	var exists bool
	return a.Deps.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM strm_entries WHERE asset_id=$1)`, id).Scan(&exists) == nil && exists
}

// Wire the selected official provider's revision into the Redis key too. A CD2
// scanner revision alone cannot invalidate a URL after official 115 changes.
func (a *API) playbackCacheVersion(ctx context.Context, ref storage.FileRef) (string, error) {
	if a.Deps.Integrations == nil {
		return "env", nil
	}
	return a.Deps.Integrations.PlaybackVersion(ctx, ref)
}
