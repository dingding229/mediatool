package httpapi

import (
	"context"
	"errors"
	"io"
	"media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/subtitles"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strconv"
	"time"
)

func (a *API) subtitles() subtitles.Repository {
	return subtitles.Repository{DB: a.Deps.DB, Root: subtitles.Root(a.Config)}
}
func subtitleError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, subtitles.ErrNotFound):
		fail(w, 404, "subtitle_not_found", e.Error())
	case errors.Is(e, subtitles.ErrConflict):
		fail(w, 409, "subtitle_conflict", e.Error())
	case errors.Is(e, subtitles.ErrInvalid):
		fail(w, 400, "invalid_subtitle", e.Error())
	default:
		fail(w, 503, "subtitle_unavailable", "字幕操作暂不可用，请刷新核对后再操作")
	}
}
func (a *API) listSubtitles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	out, e := a.subtitles().List(ctx, r.PathValue("id"))
	if e != nil {
		subtitleError(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) uploadSubtitle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, subtitles.MaxBytes+(64<<10))
	reader, e := r.MultipartReader()
	if e != nil {
		subtitleError(w, subtitles.ErrInvalid)
		return
	}
	var body []byte
	name, encoding, uploadID := "", "auto", ""
	parts := 0
	seen := map[string]bool{}
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, 413, "subtitle_too_large", "字幕文件最大5MiB，请检查文件和上传格式")
			return
		}
		parts++
		if parts > 3 || seen[part.FormName()] {
			deferutil.Report(part.Close(), "subtitle multipart close")
			subtitleError(w, subtitles.ErrInvalid)
			return
		}
		seen[part.FormName()] = true
		if part.FormName() == "file" && part.FileName() != "" && name == "" {
			name = part.FileName()
			body, e = io.ReadAll(io.LimitReader(part, subtitles.MaxBytes+1))
		} else if (part.FormName() == "encoding" || part.FormName() == "uploadId") && part.FileName() == "" {
			var raw []byte
			raw, e = io.ReadAll(io.LimitReader(part, 65))
			if len(raw) > 64 {
				e = subtitles.ErrInvalid
			}
			if part.FormName() == "uploadId" {
				uploadID = string(raw)
			} else {
				encoding = string(raw)
			}
		} else {
			e = subtitles.ErrInvalid
		}
		closeErr := part.Close()
		if e != nil || closeErr != nil {
			subtitleError(w, subtitles.ErrInvalid)
			return
		}
		if len(body) > subtitles.MaxBytes {
			fail(w, 413, "subtitle_too_large", "字幕文件最大5MiB")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, e := a.subtitles().UploadID(ctx, r.PathValue("id"), uploadID, name, encoding, body)
	if e != nil {
		subtitleError(w, e)
		return
	}
	write(w, 201, out)
}
func (a *API) saveSubtitle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var in subtitles.Metadata
	if !decode(w, r, &in) {
		return
	}
	out, e := a.subtitles().Save(ctx, r.PathValue("id"), r.PathValue("subtitleId"), in)
	if e != nil {
		subtitleError(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) deleteSubtitle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var in struct {
		Revision int64 `json:"revision"`
		Confirm  bool  `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Revision < 1 {
		subtitleError(w, subtitles.ErrInvalid)
		return
	}
	if e := a.subtitles().Delete(ctx, r.PathValue("id"), r.PathValue("subtitleId"), in.Revision); e != nil {
		subtitleError(w, e)
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) downloadSubtitle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	repo := a.subtitles()
	v, e := repo.Get(ctx, r.PathValue("id"), r.PathValue("subtitleId"))
	if e != nil {
		subtitleError(w, e)
		return
	}
	if v.Hidden || v.Deleting || v.Source == "embedded" {
		subtitleError(w, subtitles.ErrNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if v.Source == "automatic" {
		service := runtime.New(a.Deps.DB, a.Config, a.Deps.Integrations, a.Deps.Redis, a.Deps.Storage, a.Deps.Logger)
		resolver, ok := service.Links.(*runtime.Resolver)
		if !ok {
			subtitleError(w, subtitles.ErrUnavailable)
			return
		}
		target, e := resolver.ResolveSubtitle(ctx, v.AssetID, v.SourceKey, r.UserAgent(), a.observedAddress(r).ClientIP)
		if e != nil {
			subtitleError(w, subtitles.ErrNotFound)
			return
		}
		current, err := repo.Get(ctx, v.AssetID, v.ID)
		if err != nil || current.Hidden || current.Deleting {
			subtitleError(w, subtitles.ErrNotFound)
			return
		}
		w.Header().Set("Location", target.URL)
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(302)
		return
	}
	v, body, e := repo.Read(ctx, v.AssetID, v.ID)
	if e != nil {
		subtitleError(w, e)
		return
	}
	w.Header().Set("Content-Type", subtitles.ContentType(v.Extension))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", `inline; filename="subtitle.`+v.Extension+`"`)
	w.WriteHeader(200)
	if r.Method != "HEAD" {
		if _, e = w.Write(body); e != nil {
			deferutil.Report(e, "uploaded subtitle download")
		}
	}
}
func (a *API) registerSubtitles(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/media/assets/{id}/subtitles", a.protected(a.listSubtitles))
	mux.HandleFunc("POST /api/v1/media/assets/{id}/subtitles", a.protected(a.uploadSubtitle))
	mux.HandleFunc("PUT /api/v1/media/assets/{id}/subtitles/{subtitleId}", a.protected(a.saveSubtitle))
	mux.HandleFunc("DELETE /api/v1/media/assets/{id}/subtitles/{subtitleId}", a.protected(a.deleteSubtitle))
	mux.HandleFunc("GET /api/v1/media/assets/{id}/subtitles/{subtitleId}/download", a.protected(a.downloadSubtitle))
}
