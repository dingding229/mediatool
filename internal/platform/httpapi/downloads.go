package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/search"
	"net/http"
	"time"
)

func (a *API) acquisition() *acquisition.Engine {
	search := a.searchEngine()
	if search == nil {
		return nil
	}
	return &acquisition.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations, Search: *search}
}
func (a *API) listDownloads(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "downloads_unavailable", "请先配置数据库和业务连接")
		return
	}
	tasks, err := engine.List(r.Context())
	if err != nil {
		fail(w, 503, "downloads_unavailable", "下载任务暂不可用")
		return
	}
	write(w, 200, tasks)
}
func (a *API) startDownload(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "downloads_unavailable", "请先配置数据库和业务连接")
		return
	}
	var body struct {
		CorrectMediaIdentity bool   `json:"correctMediaIdentity"`
		ResourceID           string `json:"resourceId"`
		SearchRunID          string `json:"searchRunId"`
		MediaType            string `json:"mediaType"`
		TMDBID               int    `json:"tmdbId"`
	}
	if !decode(w, r, &body) {
		return
	}
	id, err := engine.StartFromRunIdentity(r.Context(), body.SearchRunID, body.ResourceID, body.MediaType, body.TMDBID, body.CorrectMediaIdentity)
	if err != nil {
		if errors.Is(err, search.ErrSnapshotUnavailable) || errors.Is(err, search.ErrSnapshotChanged) || errors.Is(err, search.ErrSnapshotExpired) {
			fail(w, 409, "search_refresh_required", err.Error())
			return
		}
		if errors.Is(err, acquisition.ErrSourceRecoveryRequired) || errors.Is(err, acquisition.ErrDownloadIdentityConflict) {
			fail(w, 409, "download_identity_or_source_conflict", err.Error())
			return
		}
		fail(w, 400, "download_create_failed", err.Error())
		return
	}
	write(w, 202, map[string]string{"id": id})
}

func (a *API) downloadSourcePreview(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "downloads_unavailable", "业务连接不可用")
		return
	}
	preview, err := engine.SourcePreview(r.Context(), r.PathValue("id"), r.URL.Query().Get("searchRunId"))
	if err != nil {
		fail(w, 409, "source_reconfirm_required", err.Error())
		return
	}
	write(w, 200, preview)
}
func (a *API) reconfirmDownloadSource(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "downloads_unavailable", "业务连接不可用")
		return
	}
	var body acquisition.SourceReconfirmInput
	if !decode(w, r, &body) {
		return
	}
	result, err := engine.ReconfirmSource(r.Context(), r.PathValue("id"), body)
	if err != nil {
		fail(w, 409, "source_reconfirm_conflict", err.Error())
		return
	}
	write(w, 202, result)
}

func (a *API) syncDownloads(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "downloads_unavailable", "下载器不可用")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	err := engine.Sync(ctx)
	message := ""
	if err != nil {
		message = err.Error()
	}
	write(w, 200, map[string]string{"message": message})
}

func (a *API) approveDownloadCleanup(w http.ResponseWriter, r *http.Request) {
	e := a.acquisition()
	if e == nil {
		fail(w, 503, "cleanup_unavailable", "数据库不可用")
		return
	}
	var in struct {
		Confirm     bool `json:"confirm"`
		HRSatisfied bool `json:"hrSatisfied"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := e.ApproveCleanup(r.Context(), r.PathValue("id"), in.Confirm, in.HRSatisfied); err != nil {
		fail(w, 409, "cleanup_blocked", err.Error())
		return
	}
	write(w, 202, map[string]bool{"queued": true})
}

func (a *API) reconcileDownloadCleanup(w http.ResponseWriter, r *http.Request) {
	e := a.acquisition()
	if e == nil {
		fail(w, 503, "cleanup_unavailable", "数据库不可用")
		return
	}
	if err := e.ReconcileCleanup(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, "cleanup_uncertain", err.Error())
		return
	}
	write(w, 200, map[string]bool{"verified": true})
}
