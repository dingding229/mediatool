package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/catalog"
	"media-tool/internal/modules/mediaserver"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/platform/auth"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type memberContextKey struct{}

func (a *API) mediaUsers() mediausers.Repository { return mediausers.Repository{DB: a.Deps.DB} }
func (a *API) mediaRegistry() *mediaserver.Registry {
	// The set is server-owned; clients cannot register a provider or change the
	// default backend. Future implementations plug into this explicit boundary.
	registry, _ := mediaserver.NewRegistry(mediaserver.Builtin{DB: a.Deps.DB}, mediaserver.EmbyAdapter{Catalog: catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}})
	return registry
}
func (a *API) mediaProviders(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"contract": mediaserver.ContractVersion, "memberProvider": "builtin", "existingPlayback": "emby-strm302-unchanged", "providers": a.mediaRegistry().Descriptions()})
}
func (a *API) nativeLibraries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	items, err := (mediaserver.Builtin{DB: a.Deps.DB}).Libraries(ctx, mediaserver.Access{})
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
		Revision int64  `json:"revision"`
		Password string `json:"password"`
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
	if err := a.mediaUsers().Mutate(ctx, r.PathValue("id"), in.Revision, action, in.Password); err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, map[string]bool{"done": true})
}
func (a *API) memberLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.Auth.SameOrigin(r) {
		fail(w, 403, "csrf_rejected", "请求来源或防跨站请求标识无效")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Username) > 128 || len(in.Password) > 512 {
		mediaError(w, mediausers.ErrCredentials)
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	name, _ := accounts.NormalizeUsername(in.Username)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	for _, limit := range []struct {
		scope string
		max   int
	}{{"global", 120}, {"ip:" + host, 20}, {"username:" + name, 10}} {
		allowed, err := a.mediaUsers().TakeAttempt(ctx, a.Config.SessionKey, limit.scope, limit.max)
		if err != nil {
			mediaError(w, err)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "媒体登录尝试过于频繁，请一分钟后重试")
			return
		}
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	token, err := mediausers.NewToken()
	if err != nil {
		mediaError(w, mediausers.ErrUnavailable)
		return
	}
	u, err := a.mediaUsers().Login(ctx, in.Username, in.Password, token)
	if err != nil {
		mediaError(w, err)
		return
	}
	mediausers.SetCookie(w, token, a.Auth.Secure)
	write(w, 200, map[string]any{"authenticated": true, "user": u})
}
func (a *API) memberProtected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		u, err := a.mediaUsers().Session(ctx, mediausers.Token(r))
		if err != nil {
			mediaError(w, err)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !a.Auth.SameOrigin(r) {
			fail(w, 403, "csrf_rejected", "请求来源或防跨站请求标识无效")
			return
		}
		if u.MustChangePassword && r.URL.Path != "/api/v1/member/auth/session" && r.URL.Path != "/api/v1/member/auth/password" && r.URL.Path != "/api/v1/member/auth/logout" {
			fail(w, 403, "password_change_required", "请先修改媒体用户初始密码")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), memberContextKey{}, u)))
	}
}
func memberUser(r *http.Request) mediausers.User {
	u, _ := r.Context().Value(memberContextKey{}).(mediausers.User)
	return u
}
func memberAccess(r *http.Request) mediaserver.Access {
	u := memberUser(r)
	return mediaserver.Access{UserID: u.ID, Revision: u.Revision}
}
func (a *API) memberSession(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"authenticated": true, "user": memberUser(r), "contract": mediaserver.ContractVersion})
}
func (a *API) memberLogout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.mediaUsers().Logout(ctx, mediausers.Token(r)); err != nil {
		mediaError(w, err)
		return
	}
	mediausers.ClearCookie(w, a.Auth.Secure)
	write(w, 200, map[string]bool{"authenticated": false})
}
func (a *API) memberPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"currentPassword"`
		Next    string `json:"newPassword"`
		Confirm string `json:"confirmPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Next != in.Confirm {
		fail(w, 400, "password_mismatch", "两次输入的密码不一致")
		return
	}
	if err := auth.ValidatePassword(in.Next); err != nil {
		mediaError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	u := memberUser(r)
	allowed, err := a.mediaUsers().TakeAttempt(ctx, a.Config.SessionKey, "password:"+u.ID, 10)
	if err != nil {
		mediaError(w, err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "密码修改尝试过于频繁，请稍后重试")
		return
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	if err = a.mediaUsers().ChangePassword(ctx, u, in.Current, in.Next); err != nil {
		mediaError(w, err)
		return
	}
	mediausers.ClearCookie(w, a.Auth.Secure)
	write(w, 200, map[string]bool{"reauthenticate": true})
}
func (a *API) memberLibraries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	items, err := (mediaserver.Builtin{DB: a.Deps.DB}).Libraries(ctx, memberAccess(r))
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, items)
}
func (a *API) memberItems(w http.ResponseWriter, r *http.Request) {
	p, err := pagination(r)
	if err != nil {
		mediaError(w, mediaserver.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	provider, err := a.mediaRegistry().Provider("builtin")
	if err != nil {
		mediaError(w, err)
		return
	}
	out, err := provider.List(ctx, memberAccess(r), mediaserver.Query{LibraryID: r.PathValue("library"), Text: r.URL.Query().Get("q"), Page: p.Page, PageSize: p.Limit})
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) memberPoster(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	path, err := (mediaserver.Builtin{DB: a.Deps.DB}).Poster(ctx, memberAccess(r), r.PathValue("library"), r.PathValue("item"))
	if err != nil {
		mediaError(w, err)
		return
	}
	// Reuse the fixed-host image loader, never forward an administrator URL/key.
	cloned := r.Clone(r.Context())
	copied := *r.URL
	copied.RawQuery = url.Values{"path": {path}}.Encode()
	cloned.URL = &copied
	a.tmdbPoster(w, cloned)
}
func (a *API) memberPlayback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	provider, err := a.mediaRegistry().Provider("builtin")
	if err != nil {
		mediaError(w, err)
		return
	}
	out, err := provider.Playback(ctx, memberAccess(r), mediaserver.PlaybackRequest{LibraryID: r.PathValue("library"), ItemID: r.PathValue("item"), UserAgent: r.UserAgent()})
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) registerMediaServer(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/media-server/providers", a.protected(a.mediaProviders))
	mux.HandleFunc("GET /api/v1/media-server/libraries", a.protected(a.nativeLibraries))
	mux.HandleFunc("GET /api/v1/media-server/users", a.protected(a.listMediaUsers))
	mux.HandleFunc("POST /api/v1/media-server/users", a.protected(a.saveMediaUser))
	mux.HandleFunc("PUT /api/v1/media-server/users/{id}", a.protected(a.saveMediaUser))
	mux.HandleFunc("DELETE /api/v1/media-server/users/{id}", a.protected(a.mutateMediaUser))
	mux.HandleFunc("POST /api/v1/media-server/users/{id}/password", a.protected(a.mutateMediaUser))
	mux.HandleFunc("POST /api/v1/media-server/users/{id}/sessions/revoke", a.protected(a.mutateMediaUser))
	mux.HandleFunc("POST /api/v1/member/auth/login", a.memberLogin)
	mux.HandleFunc("GET /api/v1/member/auth/session", a.memberProtected(a.memberSession))
	mux.HandleFunc("POST /api/v1/member/auth/logout", a.memberProtected(a.memberLogout))
	mux.HandleFunc("POST /api/v1/member/auth/password", a.memberProtected(a.memberPassword))
	mux.HandleFunc("GET /api/v1/member/libraries", a.memberProtected(a.memberLibraries))
	mux.HandleFunc("GET /api/v1/member/libraries/{library}/items", a.memberProtected(a.memberItems))
	mux.HandleFunc("GET /api/v1/member/libraries/{library}/items/{item}/poster", a.memberProtected(a.memberPoster))
	mux.HandleFunc("POST /api/v1/member/libraries/{library}/items/{item}/playback", a.memberProtected(a.memberPlayback))
}
