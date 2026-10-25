package httpapi

import (
	"context"
	"errors"
	"io"
	"media-tool/internal/modules/imageedit"
	"media-tool/internal/modules/metadataedit"
	cleanup "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (a *API) imageRepo() imageedit.Repository {
	return imageedit.Repository{DB: a.Deps.DB, Root: a.Deps.NativeImageRoot}
}
func imageFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, imageedit.ErrInvalid):
		fail(w, 400, "invalid_image", e.Error())
	case errors.Is(e, imageedit.ErrBusy):
		w.Header().Set("Retry-After", "2")
		fail(w, 429, "image_busy", e.Error())
	case errors.Is(e, metadataedit.ErrConflict):
		fail(w, 409, "image_conflict", e.Error())
	case errors.Is(e, metadataedit.ErrNotFound):
		fail(w, 404, "image_not_found", "图片或媒体条目不存在")
	default:
		fail(w, 503, "image_unavailable", "图片操作暂不可用，请重新读取后核对")
	}
}
func (a *API) mediaImages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	repo := a.imageRepo()
	id := r.PathValue("id")
	if r.Method == "GET" {
		out, e := repo.Read(ctx, id)
		if e != nil {
			imageFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	var c imageedit.Change
	var pic *imageedit.Normalized
	source := "upload"
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, imageedit.MaxBytes+(64<<10))
		if e = r.ParseMultipartForm(imageedit.MaxBytes + (64 << 10)); e != nil {
			imageFailure(w, imageedit.ErrInvalid)
			return
		}
		defer cleanup.Do(r.MultipartForm.RemoveAll, "image multipart cleanup")
		if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
			imageFailure(w, imageedit.ErrInvalid)
			return
		}
		for key, values := range r.MultipartForm.Value {
			if (key != "kind" && key != "targetKey" && key != "revision") || len(values) != 1 {
				imageFailure(w, imageedit.ErrInvalid)
				return
			}
		}

		c.TargetKey, c.Kind = r.FormValue("targetKey"), r.FormValue("kind")
		c.Revision, e = strconv.ParseInt(r.FormValue("revision"), 10, 64)
		if e != nil || !imageedit.ValidKind(c.Kind) {
			imageFailure(w, imageedit.ErrInvalid)
			return
		}
		// Authenticate/resolve before pixel work; stale revisions are rechecked inside Save.
		if _, e = repo.Read(ctx, id); e != nil {
			imageFailure(w, e)
			return
		}
		f, h, e := r.FormFile("file")
		if e != nil {
			imageFailure(w, imageedit.ErrInvalid)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(f, imageedit.MaxBytes+1))
		closeErr := f.Close()
		if e != nil || closeErr != nil {
			imageFailure(w, imageedit.ErrInvalid)
			return
		}
		normalized, e := imageedit.Normalize(ctx, h.Filename, raw)
		if e != nil {
			imageFailure(w, e)
			return
		}
		pic = &normalized
	} else {
		var in struct {
			imageedit.Change
			Path string `json:"path"`
		}
		if !decodeLimit(w, r, &in, 4096) {
			return
		}
		c = in.Change
		if in.Path != "" {
			source = "tmdb"
			paths, e := a.imageCandidates(ctx, id)
			if e != nil {
				imageFailure(w, e)
				return
			}
			allowed := false
			for _, p := range paths[c.Kind] {
				if p == in.Path {
					allowed = true
				}
			}
			if !allowed {
				imageFailure(w, imageedit.ErrInvalid)
				return
			}
			client, e := a.Deps.Integrations.HTTPClient(ctx, "tmdb")
			if e != nil {
				imageFailure(w, e)
				return
			}
			safe := *client
			safe.Timeout = 12 * time.Second
			safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			req, e := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/original"+in.Path, nil)
			if e != nil {
				imageFailure(w, e)
				return
			}
			res, e := safe.Do(req)
			if e != nil {
				imageFailure(w, e)
				return
			}
			raw, e := io.ReadAll(io.LimitReader(res.Body, imageedit.MaxBytes+1))
			closeErr := res.Body.Close()
			if e != nil || closeErr != nil || res.StatusCode != 200 {
				imageFailure(w, imageedit.ErrInvalid)
				return
			}
			normalized, e := imageedit.Normalize(ctx, in.Path, raw)
			if e != nil {
				imageFailure(w, e)
				return
			}
			pic = &normalized
		}
	}
	out, e := repo.Save(ctx, id, account.ID, c, pic, source)
	if e != nil {
		imageFailure(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) imageCandidates(ctx context.Context, id string) (map[string][]string, error) {
	if a.Deps.Integrations == nil {
		return nil, metadataedit.ErrUnavailable
	}
	record, e := a.imageRepo().Read(ctx, id)
	if e != nil {
		return nil, e
	}
	var tmdb int64
	if e = a.Deps.DB.QueryRow(ctx, `SELECT tmdb_id FROM media_items WHERE id=$1`, record.Target.MediaID).Scan(&tmdb); e != nil {
		return nil, e
	}
	client, e := a.Deps.Integrations.TMDB(ctx)
	if e != nil {
		return nil, e
	}
	t := record.Target
	return client.ImageCandidates(ctx, t.Kind, tmdb, t.Season, t.Episode)
}
func (a *API) mediaImageCandidates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	paths, e := a.imageCandidates(ctx, r.PathValue("id"))
	if e != nil {
		imageFailure(w, e)
		return
	}
	type candidate struct {
		Path string `json:"path"`
		URL  string `json:"url"`
	}
	out := map[string][]candidate{}
	for k, list := range paths {
		out[k] = []candidate{}
		for _, p := range list {
			out[k] = append(out[k], candidate{p, "/api/v1/media/tmdb/image?path=" + url.QueryEscape(p)})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) mediaImage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	kind := r.PathValue("kind")
	if !imageedit.ValidKind(kind) {
		http.NotFound(w, r)
		return
	}
	repo := a.imageRepo()
	record, e := repo.Read(ctx, r.PathValue("id"))
	if e != nil {
		imageFailure(w, e)
		return
	}
	fid := record.Overrides[kind]
	if fid == "" {
		if base, ok := record.Base[kind]; ok {
			http.Redirect(w, r, base.URL, 302)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	raw, ct, tag, e := repo.File(ctx, fid)
	if e != nil {
		imageFailure(w, e)
		return
	}
	serveEditedImage(w, r, raw, ct, tag)
}
func serveEditedImage(w http.ResponseWriter, r *http.Request, raw []byte, ct, tag string) {
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("ETag", strconv.Quote(tag))
	if r.Header.Get("If-None-Match") == strconv.Quote(tag) {
		w.WriteHeader(304)
		return
	}
	_, e := w.Write(raw)
	cleanup.Report(e, "edited image response")
}
