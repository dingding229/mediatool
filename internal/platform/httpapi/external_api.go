package httpapi

import (
	"errors"
	"media-tool/internal/modules/externalapi"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/buildinfo"
	"net/http"
	"strconv"
	"strings"
)

func (a *API) apiKeys() externalapi.Repository {
	return externalapi.Repository{DB: a.Deps.DB, Signer: a.Deps.ExternalSigner}
}
func (a *API) externalReady(w http.ResponseWriter) bool {
	if a.Deps.DB == nil || a.Deps.ExternalSigner == nil {
		fail(w, 503, "api_unavailable", "外部API凭据服务暂不可用")
		return false
	}
	return true
}
func (a *API) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !a.externalReady(w) {
		return
	}
	out, e := a.apiKeys().List(r.Context())
	if e != nil {
		fail(w, 503, "api_keys_unavailable", "凭据列表读取失败")
		return
	}
	write(w, 200, out)
}
func (a *API) externalActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	v, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return "", false
	}
	return v.Username, true
}
func (a *API) createAPIKey(w http.ResponseWriter, r *http.Request) {
	if !a.externalReady(w) {
		return
	}
	var in struct {
		Name    string   `json:"name"`
		Scopes  []string `json:"scopes"`
		Confirm bool     `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请确认创建独立API凭据")
		return
	}
	actor, ok := a.externalActor(w, r)
	if !ok {
		return
	}
	out, e := a.apiKeys().Create(r.Context(), in.Name, in.Scopes, actor)
	if e != nil {
		fail(w, 400, "api_key_invalid", "凭据创建失败，请检查名称和权限")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 201, out)
}
func (a *API) revealAPIKey(w http.ResponseWriter, r *http.Request) {
	if !a.externalReady(w) {
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Action != "view" && in.Action != "copy" {
		fail(w, 400, "api_key_invalid", "仅支持查看或复制")
		return
	}
	actor, ok := a.externalActor(w, r)
	if !ok {
		return
	}
	out, e := a.apiKeys().Reveal(r.Context(), r.PathValue("id"), actor, in.Action)
	if errors.Is(e, externalapi.ErrRevoked) {
		fail(w, 409, "api_key_revoked", e.Error())
		return
	}
	if e != nil {
		fail(w, 503, "api_key_reveal_failed", "凭据未披露，请刷新后重试")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) revokeAPIKey(w http.ResponseWriter, r *http.Request) { a.changeAPIKey(w, r, false) }
func (a *API) deleteAPIKey(w http.ResponseWriter, r *http.Request) { a.changeAPIKey(w, r, true) }
func (a *API) changeAPIKey(w http.ResponseWriter, r *http.Request, remove bool) {
	if !a.externalReady(w) {
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请明确确认此操作")
		return
	}
	actor, ok := a.externalActor(w, r)
	if !ok {
		return
	}
	var e error
	if remove {
		e = a.apiKeys().Delete(r.Context(), r.PathValue("id"), actor)
	} else {
		e = a.apiKeys().Revoke(r.Context(), r.PathValue("id"), actor)
	}
	if e != nil {
		fail(w, 409, "api_key_invalid", "操作未完成：删除前须先撤销，请刷新后核对")
		return
	}
	write(w, 200, map[string]bool{"ok": true, "revoked": !remove, "deleted": remove})
}
func (a *API) listAPICalls(w http.ResponseWriter, r *http.Request) {
	if !a.externalReady(w) {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			fail(w, 400, "invalid_limit", "调用记录条数须为1–100")
			return
		}
		limit = v
	}
	out, e := a.apiKeys().Calls(r.Context(), r.URL.Query().Get("keyId"), limit)
	if e != nil {
		fail(w, 503, "api_calls_unavailable", "调用记录读取失败")
		return
	}
	write(w, 200, out)
}

type externalResponse struct {
	http.ResponseWriter
	status int
}

func (w *externalResponse) WriteHeader(n int) { w.status = n; w.ResponseWriter.WriteHeader(n) }
func (w *externalResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
func (a *API) external(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a.Deps.DB == nil || a.Deps.ExternalSigner == nil {
			fail(w, 503, "api_unavailable", "数据库不可用")
			return
		}
		parts := strings.Split(r.Header.Get("Authorization"), " ")
		if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			fail(w, 401, "api_unauthorized", "请使用Authorization: Bearer独立API凭据")
			return
		}
		keyID, call, err := a.apiKeys().Admit(r.Context(), parts[1], scope, r.Method, externalLogText(r.URL.Path))
		if err != nil {
			status := 401
			if errors.Is(err, externalapi.ErrForbidden) {
				status = 403
			}
			if errors.Is(err, externalapi.ErrUnavailable) {
				status = 503
			}
			if errors.Is(err, externalapi.ErrRate) {
				status = 429
			}
			fail(w, status, "api_access_denied", err.Error())
			return
		}
		out := &externalResponse{ResponseWriter: w}
		defer func() {
			status := out.status
			if status == 0 {
				status = 200
			}
			evidence, ok := r.Context().Value(ipEvidenceKey{}).(ipEvidence)
			ip, source := "", "unknown"
			if ok && evidence.err == nil {
				ip = evidence.address.ClientIP
				source = evidence.address.Source
			}
			a.Deps.ExternalCalls.Submit(externalapi.Observation{KeyID: keyID, CallID: call, IP: ip, Source: source, Status: status})
		}()
		if r.Method != "GET" && r.Method != "HEAD" {
			if paused, err := maintenance.Paused(r.Context(), a.Deps.DB); err != nil || paused {
				fail(w, 423, "maintenance_paused", "维护期间暂停外部业务写入")
				return
			}
		}
		next(out, r)
	}
}
func (a *API) registerExternal(mux routeRegistrar) {
	mux.HandleFunc("POST /api/v1/external-api/keys/{id}/reveal", a.protected(a.revealAPIKey))
	mux.HandleFunc("DELETE /api/v1/external-api/keys/{id}", a.protected(a.deleteAPIKey))
	mux.HandleFunc("GET /api/v1/external-api/keys", a.protected(a.listAPIKeys))
	mux.HandleFunc("POST /api/v1/external-api/keys", a.protected(a.createAPIKey))
	mux.HandleFunc("POST /api/v1/external-api/keys/{id}/revoke", a.protected(a.revokeAPIKey))
	mux.HandleFunc("GET /api/v1/external-api/calls", a.protected(a.listAPICalls))
	mux.HandleFunc("GET /api/external/v1/status", a.external("status.read", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"name": "Media Tool", "version": buildinfo.Version, "databaseConfigured": a.Deps.DB != nil})
	}))
	mux.HandleFunc("GET /api/external/v1/classification", a.external("layout.preview", a.getClassification))
	mux.HandleFunc("POST /api/external/v1/layout/preview", a.external("layout.preview", a.previewClassification))
	mux.HandleFunc("GET /api/external/v1/strm", a.external("library.read", a.managedOutputs))
	mux.HandleFunc("GET /api/external/v1/strm/{id}/content", a.external("library.read", a.managedOutputContent))
	mux.HandleFunc("GET /api/external/v1/downloads", a.external("tasks.read", a.externalDownloads))
	mux.HandleFunc("GET /api/external/v1/transfers", a.external("tasks.read", a.listTransfers))
	mux.HandleFunc("POST /api/external/v1/search", a.external("search.run", a.startSearch))
	mux.HandleFunc("GET /api/external/v1/search/{id}", a.external("search.run", a.getSearch))
	mux.HandleFunc("POST /api/external/v1/downloads", a.external("downloads.run", a.startDownload))
}

func (a *API) externalDownloads(w http.ResponseWriter, r *http.Request) {
	e := a.acquisition()
	if e == nil {
		fail(w, 503, "downloads_unavailable", "数据库不可用")
		return
	}
	tasks, err := e.List(r.Context())
	if err != nil {
		fail(w, 503, "downloads_unavailable", err.Error())
		return
	}
	out := []map[string]any{}
	for _, t := range tasks {
		out = append(out, map[string]any{"id": t.ID, "title": t.Title, "state": t.State, "progress": t.Progress, "qbState": t.QBState, "downloadSpeed": t.DownloadSpeed, "syncedAt": t.SyncedAt, "syncError": t.SyncError, "media": t.Media, "cleanupState": t.CleanupState})
	}
	write(w, 200, out)
}
