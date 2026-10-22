package httpapi

import (
	"context"
	"io"
	"media-tool/internal/modules/manualdownloads"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strings"
	"time"
)

type manualDownloadRetryInput struct {
	Confirm             bool `json:"confirm"`
	AcceptDuplicateRisk bool `json:"acceptDuplicateRisk"`
}

func (a *API) manualDownloads() manualdownloads.Engine {
	return manualdownloads.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
}
func (a *API) listManualDownloads(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	if servePage(w, r, a.Deps.DB, `SELECT count(*) FROM manual_download_tasks`, nil, func(start, limit int) ([]manualdownloads.Task, error) {
		return a.manualDownloads().Page(r.Context(), start, limit, "", "", nil)
	}) {
		return
	}
	out, err := a.manualDownloads().List(r.Context())
	if err != nil {
		fail(w, 503, "downloads_unavailable", "手动任务不可用")
		return
	}
	write(w, 200, out)
}
func (a *API) createManualDownload(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var source manualdownloads.Source
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, manualdownloads.MaxTorrent+(1<<20))
		if err = r.ParseMultipartForm(1 << 20); err != nil {
			fail(w, 400, "invalid_torrent", "种子表单无效或超过10MiB")
			return
		}
		defer deferutil.Do(func() error { return r.MultipartForm.RemoveAll() }, "Temporary cleanup or final unlock only; resource close also releases locks and the primary result is authoritative")
		file, header, openErr := r.FormFile("torrent")
		if openErr != nil {
			fail(w, 400, "invalid_torrent", "请选择torrent文件")
			return
		}
		defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".torrent") {
			fail(w, 400, "invalid_torrent", "请选择.torrent文件")
			return
		}
		var raw []byte
		raw, err = io.ReadAll(io.LimitReader(file, manualdownloads.MaxTorrent+1))
		if err == nil {
			source, err = manualdownloads.File(raw, header.Filename)
		}
	} else {
		var in struct {
			URL string `json:"url"`
		}
		if !decode(w, r, &in) {
			return
		}
		source, err = manualdownloads.Link(ctx, in.URL)
	}
	if err != nil {
		fail(w, 400, "invalid_torrent", err.Error())
		return
	}
	id, err := a.manualDownloads().CreateWithImport(ctx, source)
	if err != nil {
		fail(w, 400, "download_create_failed", err.Error())
		return
	}
	a.Deps.Logger.Info("manual torrent queued", "module", "downloads", "taskId", id, "sourceKind", source.Kind, "source", source.Label, "infoHash", source.Meta.InfoHash)
	write(w, 202, map[string]string{"id": id})
}

func (a *API) retryManualDownload(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "任务ID无效")
		return
	}
	var in manualDownloadRetryInput
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.manualDownloads().Retry(ctx, r.PathValue("id"), in.Confirm, in.AcceptDuplicateRisk); err != nil {
		fail(w, 409, "manual_retry_blocked", err.Error())
		return
	}
	write(w, 202, map[string]bool{"queued": true})
}
func (a *API) reconcileManualDownload(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "任务ID无效")
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	state, err := a.manualDownloads().Reconcile(ctx, r.PathValue("id"))
	if err != nil {
		fail(w, 409, "manual_reconcile_failed", err.Error())
		return
	}
	write(w, 200, map[string]string{"state": state})
}

func (a *API) importManualDownload(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "任务ID无效")
		return
	}
	var in struct {
		Confirm   bool   `json:"confirm"`
		MediaType string `json:"mediaType"`
		TMDBID    int    `json:"tmdbId"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := a.manualDownloads().ConfirmImport(ctx, r.PathValue("id"), in.MediaType, in.TMDBID, in.Confirm); err != nil {
		fail(w, 409, "manual_import_blocked", err.Error())
		return
	}
	write(w, 202, map[string]bool{"queued": true})
}
