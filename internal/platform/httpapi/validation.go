package httpapi

import (
	"net/http"
)

// Playback preflight reports effective runtime configuration, never credentials.
func (a *API) playbackPreflight(w http.ResponseWriter, r *http.Request) {
	cloud := a.storageConfigured(r.Context())
	source := "none"
	if cloud {
		source = "environment"
	}
	if a.Deps.Integrations != nil {
		if _, err := a.Deps.Integrations.Repository.Default(r.Context(), "clouddrive2"); err == nil {
			source = "database"
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"cloudConfigured": cloud, "cloudSource": source, "playbackResolveTimeoutMs": a.Engine.ResolveBudget().Milliseconds(), "legacyKeyEnabled": a.Config.LegacyPlaybackEnabled, "realPlayerVerified": false})
}

// Explicit authenticated download; listing assets never returns file keys.
func (a *API) assetValidationRef(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var path string
	if a.Deps.DB.QueryRow(r.Context(), `SELECT f.full_path FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 AND f.state='present'`, r.PathValue("id")).Scan(&path) != nil {
		fail(w, 404, "asset_unavailable", "文件不存在或未确认可用")
		return
	}
	write(w, 200, map[string]string{"path": path})
}
