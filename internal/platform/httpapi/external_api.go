package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/externalapi"
	"net/http"
	"strings"
	"time"
)

func (a *API) apiKeys() externalapi.Repository {
	return externalapi.Repository{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}
}
func (a *API) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	out, err := a.apiKeys().List(r.Context())
	if err != nil {
		fail(w, 503, "api_keys_unavailable", err.Error())
		return
	}
	write(w, 200, out)
}
func (a *API) createAPIKey(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		Name      string    `json:"name"`
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expiresAt"`
		Confirm   bool      `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请明确确认创建独立API凭据")
		return
	}
	out, err := a.apiKeys().Create(r.Context(), in.Name, in.Scopes, in.ExpiresAt)
	if err != nil {
		fail(w, 400, "api_key_invalid", err.Error())
		return
	}
	write(w, 201, out)
}
func (a *API) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请确认撤销此API凭据")
		return
	}
	if err := a.apiKeys().Revoke(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 400, "api_key_invalid", err.Error())
		return
	}
	write(w, 200, map[string]bool{"revoked": true})
}
func (a *API) listAPICalls(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT k.name,c.method,c.path,c.status,c.created_at FROM external_api_calls c LEFT JOIN external_api_keys k ON k.id=c.key_id ORDER BY c.id DESC LIMIT 100`)
	if err != nil {
		fail(w, 503, "api_calls_unavailable", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, method, path string
		var status int
		var at time.Time
		if rows.Scan(&name, &method, &path, &status, &at) != nil {
			fail(w, 503, "api_calls_unavailable", "调用记录读取失败")
			return
		}
		out = append(out, map[string]any{"name": name, "method": method, "path": path, "status": status, "createdAt": at})
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
		if a.Deps.DB == nil || a.Deps.Integrations == nil {
			fail(w, 503, "api_unavailable", "数据库不可用")
			return
		}
		parts := strings.Split(r.Header.Get("Authorization"), " ")
		if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			fail(w, 401, "api_unauthorized", "请使用Authorization: Bearer独立API凭据")
			return
		}
		_, call, err := a.apiKeys().Admit(r.Context(), parts[1], scope, r.Method, r.URL.Path)
		if err != nil {
			status := 401
			if errors.Is(err, externalapi.ErrForbidden) {
				status = 403
			}
			if errors.Is(err, externalapi.ErrRate) {
				status = 429
			}
			fail(w, status, "api_access_denied", err.Error())
			return
		}
		out := &externalResponse{ResponseWriter: w}
		defer func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			a.Deps.DB.Exec(ctx, `UPDATE external_api_calls SET status=$2 WHERE id=$1`, call, out.status)
			a.Deps.DB.Exec(ctx, `DELETE FROM external_api_calls WHERE id IN(SELECT id FROM external_api_calls ORDER BY id DESC OFFSET 10000)`)
		}()
		next(out, r)
	}
}
func (a *API) registerExternal(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/external-api/keys", a.protected(a.listAPIKeys))
	mux.HandleFunc("POST /api/v1/external-api/keys", a.protected(a.createAPIKey))
	mux.HandleFunc("POST /api/v1/external-api/keys/{id}/revoke", a.protected(a.revokeAPIKey))
	mux.HandleFunc("GET /api/v1/external-api/calls", a.protected(a.listAPICalls))
	mux.HandleFunc("GET /api/external/v1/status", a.external("status.read", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"name": "Media Tool", "version": "0.25.0-dev", "databaseConfigured": a.Deps.DB != nil})
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
