package httpapi

import (
	"context"
	"io"
	"media-tool/internal/modules/manualdownloads"
	"net/http"
	"strings"
	"time"
)

func (a *API) manualDownloads() manualdownloads.Engine {
	return manualdownloads.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
}
func (a *API) listManualDownloads(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
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
		defer r.MultipartForm.RemoveAll()
		file, header, openErr := r.FormFile("torrent")
		if openErr != nil {
			fail(w, 400, "invalid_torrent", "请选择torrent文件")
			return
		}
		defer file.Close()
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
	id, err := a.manualDownloads().Create(ctx, source)
	if err != nil {
		fail(w, 400, "download_create_failed", err.Error())
		return
	}
	a.Deps.Logger.Info("manual torrent queued", "module", "downloads", "taskId", id, "sourceKind", source.Kind, "source", source.Label, "infoHash", source.Meta.InfoHash)
	write(w, 202, map[string]string{"id": id})
}
