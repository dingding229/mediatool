package httpapi

import (
	"errors"
	"io"
	"media-tool/internal/modules/integrations"
	"net/http"
	"os"
)

// Playback preflight reports effective runtime configuration, never credentials.
func (a *API) playbackPreflight(w http.ResponseWriter, r *http.Request) {
	cloud, embyReady := a.storageConfigured(r.Context()), false
	cloudSource, embySource := "none", "none"
	enabled := false
	if cloud {
		cloudSource = "environment"
	}
	if a.Deps.Integrations != nil {
		if _, err := a.Deps.Integrations.Repository.Default(r.Context(), "clouddrive2"); err == nil {
			cloudSource = "database"
		}
		_, err := a.Deps.Integrations.Repository.Default(r.Context(), "emby")
		if err == nil {
			embySource = "database"
			embyReady = true
		} else if !errors.Is(err, integrations.ErrNotFound) {
			fail(w, 503, "preflight_unavailable", "无法读取有效服务配置")
			return
		} else {
			var exists bool
			if a.Deps.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM integrations WHERE kind='emby')`).Scan(&exists) != nil {
				fail(w, 503, "preflight_unavailable", "无法读取配置")
				return
			}
			if !exists && a.Gateway != nil {
				embySource = "environment"
				embyReady = true
			}
		}
	} else if a.Gateway != nil {
		embySource = "environment"
		embyReady = true
	}
	if embyReady {
		if g := a.gateway(r.Context()); g != nil {
			enabled = g.RedirectEnabled()
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"cloudConfigured": cloud, "cloudSource": cloudSource, "embyConfigured": embyReady, "embySource": embySource, "gatewayEnabled": enabled, "playbackResolveTimeoutMs": a.Engine.ResolveBudget().Milliseconds(), "legacyKeyEnabled": a.Config.LegacyPlaybackEnabled, "realPlayerVerified": false})
}

// Explicit authenticated download; listing assets never returns file keys.
func (a *API) assetSTRM(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var output string
	if a.Deps.DB.QueryRow(r.Context(), `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path LIMIT 1`, r.PathValue("id")).Scan(&output) != nil {
		fail(w, 404, "strm_unavailable", "没有对应STRM")
		return
	}
	root, err := os.OpenRoot(a.Config.STRMOutputDir)
	if err != nil {
		fail(w, 503, "strm_unavailable", "STRM目录不可用")
		return
	}
	defer root.Close()
	f, err := root.Open(output)
	if err != nil {
		fail(w, 404, "strm_unavailable", "STRM文件不可用")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(raw) > 8192 {
		fail(w, 409, "strm_invalid", "STRM无效")
		return
	}
	if string(raw) == "" {
		fail(w, 409, "strm_invalid", "STRM无效")
		return
	}
	// Do not return a hand-edited URL containing an unrelated secret.
	g := a.gateway(r.Context())
	if g == nil || !g.ManagedLink(r.Context(), string(raw)) {
		fail(w, 409, "strm_unmanaged", "STRM未通过托管验证")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="media.strm"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(raw)
}

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
