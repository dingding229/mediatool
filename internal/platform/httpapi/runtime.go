package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/emby"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *API) builder(ctx context.Context) strm.Builder {
	b := a.Builder
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
func (a *API) gateway(ctx context.Context) *emby.Gateway {
	if a.Deps.Integrations != nil {
		if c, err := a.Deps.Integrations.Repository.Default(ctx, "emby"); err == nil {
			target, err := url.Parse(c.String("endpoint"))
			if err != nil {
				return nil
			}
			enabled, _ := c.Record.Settings["gatewayEnabled"].(bool)
			return emby.NewGateway(emby.GatewayOptions{Identities: a.EmbyIdentities, Upstream: target, PublicOrigin: embyPublicOrigin(a.Config.EmbyPublicURL), Enable302: enabled, DirectPlayOnly: a.directPlayOnly(ctx), LegacyEnabled: a.Config.LegacyPlaybackEnabled, OwnedAsset: a.ownedAsset, Builder: a.builder(ctx), STRMOutputDir: a.Config.STRMOutputDir, STRMPathPrefix: c.String("strmPathPrefix"), ResolverOrigins: a.Config.EmbyResolverOrigins, Playback: a.Engine, ManagedURL: a.grants().VerifyManagedURL})
		}
		var exists bool
		if a.Deps.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE kind='emby')`).Scan(&exists) != nil || exists {
			return nil
		}
	}
	return a.Gateway
}
func (a *API) proxyEmby(w http.ResponseWriter, r *http.Request) {
	gateway := a.gateway(r.Context())
	if gateway == nil {
		fail(w, 503, "emby_disabled", "请配置并启用 Emby 连接")
		return
	}
	if visit, ok := r.Context().Value(playerVisitKey{}).(*playerVisit); ok && visit.ID != "" {
		identity, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		id, name, err := gateway.VerifiedCaller(identity, r)
		cancel()
		if err == nil {
			visit.UserID = id
			visit.UserName = name
			visit.Verified = true
		}
	}
	gateway.ServeHTTP(w, r)
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

// EmbyHandler serves the gateway at its own origin/root, leaving the management
// SPA on a separate listener. Clone the URL before adding the internal mount.
func EmbyHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		copy := r.Clone(emby.WithRootOrigin(r.Context()))
		u := *r.URL
		u.Path = r.URL.Path
		if !strings.HasPrefix(u.Path, "/emby/") && u.Path != "/emby" {
			u.Path = "/emby" + u.Path
		}
		u.RawPath = ""
		copy.URL = &u
		handler.ServeHTTP(w, copy)
	})
}

func embyPublicOrigin(raw string) *url.URL {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return u
}

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

func (a *API) directPlayOnly(ctx context.Context) bool {
	if a.Deps.Integrations != nil {
		if c, err := a.Deps.Integrations.Repository.Default(ctx, "emby"); err == nil {
			if value, ok := c.Record.Settings["directPlayOnly"].(bool); ok {
				return value
			}
		} else if !errors.Is(err, integrations.ErrNotFound) {
			// Unknown live policy cannot authorize a media relay.
			return true
		}
	}
	return a.Config.EmbyDirectPlayOnly
}
