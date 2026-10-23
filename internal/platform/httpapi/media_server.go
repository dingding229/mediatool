package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/catalog"
	"media-tool/internal/modules/mediaserver"
	"media-tool/internal/modules/mediausers"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/platform/auth"
	"net/http"
	"strings"
	"time"
)

func (a *API) mediaUsers() mediausers.Repository { return mediausers.Repository{DB: a.Deps.DB} }
func (a *API) mediaRegistry() *mediaserver.Registry {
	// The set is server-owned; clients cannot register a provider or change the
	// default backend. Future implementations plug into this explicit boundary.
	registry, _ := mediaserver.NewRegistry(mediaserver.Builtin{DB: a.Deps.DB}, mediaserver.EmbyAdapter{Catalog: catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}})
	return registry
}
func (a *API) mediaProviders(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"contract": mediaserver.ContractVersion, "mediaUserModel": "builtin", "existingPlayback": "emby-strm302-unchanged", "providers": a.mediaRegistry().Descriptions()})
}
func (a *API) nativeLibraries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	page, err := (nativecatalog.Repository{DB: a.Deps.DB}).List(ctx, 0, 200)
	items := []mediaserver.Library{}
	for _, l := range page.Items {
		if l.Hidden {
			continue
		}
		kind := "movie"
		if l.CollectionType == "tvshows" {
			kind = "series"
		}
		items = append(items, mediaserver.Library{ID: l.ID, Name: l.Name, MediaType: kind})
	}
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, items)
}
func mediaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mediausers.ErrCredentials):
		fail(w, 401, "invalid_credentials", "账号或密码不正确，或媒体会话已失效")
	case errors.Is(err, mediausers.ErrConflict):
		fail(w, 409, "media_user_conflict", "账号重复或数据已变更，请刷新后重试")
	case errors.Is(err, mediaserver.ErrDenied):
		fail(w, 403, "library_denied", "没有此媒体库的访问权限")
	case errors.Is(err, mediaserver.ErrNotFound), errors.Is(err, mediausers.ErrNotFound):
		fail(w, 404, "media_not_found", "媒体或用户不存在")
	case errors.Is(err, mediaserver.ErrUnsupported):
		fail(w, 501, "capability_reserved", "内置媒体库播放授权接口已预留，尚未实现；现有Emby播放不受影响")
	case errors.Is(err, mediausers.ErrUnavailable), errors.Is(err, mediaserver.ErrUnavailable):
		fail(w, 503, "media_server_unavailable", "媒体服务暂不可用")
	default:
		fail(w, 400, "invalid_media_request", err.Error())
	}
}
func (a *API) listMediaUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if a.Deps.DB != nil && servePage(w, r, a.Deps.DB, `SELECT count(*) FROM media_users`, nil, func(start, limit int) ([]mediausers.User, error) { return a.mediaUsers().AdminPage(ctx, start, limit) }) {
		return
	}
	out, err := a.mediaUsers().List(ctx)
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) saveMediaUser(w http.ResponseWriter, r *http.Request) {
	var in mediausers.Input
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if err := in.Validate(id == ""); err != nil {
		mediaError(w, err)
		return
	}
	if id == "" {
		release := passwordPermit(w)
		if release == nil {
			return
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	u, err := a.mediaUsers().Save(ctx, id, in)
	if err != nil {
		mediaError(w, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	write(w, status, u)
}
func (a *API) mutateMediaUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision           int64  `json:"revision"`
		Password           string `json:"password"`
		MustChangePassword bool   `json:"mustChangePassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	action := "delete"
	if strings.HasSuffix(r.URL.Path, "/password") {
		action = "password"
		if err := auth.ValidatePassword(in.Password); err != nil {
			mediaError(w, err)
			return
		}
		release := passwordPermit(w)
		if release == nil {
			return
		}
		defer release()
	} else if strings.HasSuffix(r.URL.Path, "/sessions/revoke") {
		action = "revoke"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.mediaUsers().Mutate(ctx, r.PathValue("id"), in.Revision, action, in.Password, in.MustChangePassword); err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, map[string]bool{"done": true})
}
func (a *API) registerMediaServer(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/media-server/native/library-mappings", a.protected(a.nativeLibraryMappings))
	mux.HandleFunc("GET /api/v1/media-server/users/{id}/devices", a.protected(a.mediaUserDevices))
	mux.HandleFunc("POST /api/v1/media-server/users/{id}/devices/{deviceId}/revoke", a.protected(a.revokeMediaDevice))
	mux.HandleFunc("GET /api/v1/media-server/users/{id}/history", a.protected(a.mediaUserHistory))
	mux.HandleFunc("GET /api/v1/media-server/playback-statistics", a.protected(a.playbackStatistics))
	mux.HandleFunc("GET /api/v1/media-server/playback-events", a.protected(a.playbackEvents))
	mux.HandleFunc("GET /api/v1/media-server/device-ips", a.protected(a.deviceIPHistory))
	mux.HandleFunc("GET /api/v1/media-server/playback-ips", a.protected(a.playbackIPs))
	mux.HandleFunc("GET /api/v1/media-server/native/settings", a.protected(a.nativeServerSettings))
	mux.HandleFunc("PUT /api/v1/media-server/native/settings", a.protected(a.nativeServerSettings))
	mux.HandleFunc("GET /api/v1/media-server/native/processing", a.protected(a.processingSummary))
	mux.HandleFunc("PUT /api/v1/media-server/native/processing/settings", a.protected(a.processingSettings))
	mux.HandleFunc("GET /api/v1/media-server/native/processing/tasks", a.protected(a.processingTasks))
	mux.HandleFunc("POST /api/v1/media-server/native/processing/retry", a.protected(a.processingRetry))
	mux.HandleFunc("POST /api/v1/media-server/native/processing/gc", a.protected(a.processingGC))
	mux.HandleFunc("POST /api/v1/media-server/native/probe/{assetId}/retry", a.protected(a.retryNativeProbe))
	mux.HandleFunc("POST /api/v1/media-server/native/tmdb/refresh", a.protected(a.refreshNativeTMDB))
	mux.HandleFunc("GET /api/v1/media-server/native/libraries", a.protected(a.listNativeLibraries))
	mux.HandleFunc("GET /api/v1/media-server/providers", a.protected(a.mediaProviders))
	mux.HandleFunc("GET /api/v1/media-server/libraries", a.protected(a.nativeLibraries))
	mux.HandleFunc("GET /api/v1/media-server/users", a.protected(a.listMediaUsers))
	mux.HandleFunc("POST /api/v1/media-server/users", a.protected(a.saveMediaUser))
	mux.HandleFunc("GET /api/v1/media-server/users/{id}", a.protected(a.getMediaUser))
	mux.HandleFunc("PUT /api/v1/media-server/users/{id}", a.protected(a.saveMediaUser))
	mux.HandleFunc("DELETE /api/v1/media-server/users/{id}", a.protected(a.mutateMediaUser))
	mux.HandleFunc("POST /api/v1/media-server/users/{id}/password", a.protected(a.mutateMediaUser))
	mux.HandleFunc("POST /api/v1/media-server/users/{id}/sessions/revoke", a.protected(a.mutateMediaUser))
}

func (a *API) getMediaUser(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	u, err := a.mediaUsers().Get(ctx, r.PathValue("id"))
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, u)
}
