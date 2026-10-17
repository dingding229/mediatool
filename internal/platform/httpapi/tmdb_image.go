package httpapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"regexp"
	"time"
)

var tmdbImageFile = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}\.(?:jpg|png|webp)$`)

func (a *API) tmdbImage(w http.ResponseWriter, r *http.Request) {
	size, file := r.PathValue("size"), r.PathValue("file")
	if !tmdbImageFile.MatchString(file) || (size != "w185" && size != "w342" && size != "w500" && size != "w1280") {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	key := fmt.Sprintf("media-tool:tmdb-image-cache:%x", sha256.Sum256([]byte(size+"/"+file)))
	const maxImage = 4 << 20
	var raw []byte
	if a.Deps.Redis != nil {
		small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		length, e := a.Deps.Redis.StrLen(small, key).Result()
		if e == nil && length > 0 && length <= maxImage {
			raw, _ = a.Deps.Redis.GetRange(small, key, 0, maxImage).Bytes()
		}
		stop()
	}
	if len(raw) == 0 || len(raw) > maxImage {
		client := http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if a.Deps.Integrations != nil {
			selected, e := a.Deps.Integrations.HTTPClient(ctx, "tmdb")
			if e != nil {
				http.Error(w, "image proxy unavailable", 502)
				return
			}
			client = *selected
			client.Timeout = 8 * time.Second
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		}

		// Fixed public image origin only. Never attach TMDB token, PT Cookie, client UA or headers.
		request, e := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/"+size+"/"+file, nil)
		if e != nil {
			http.Error(w, "image unavailable", 502)
			return
		}
		response, e := client.Do(request)
		if e != nil {
			http.Error(w, "image unavailable", 502)
			return
		}
		defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if response.StatusCode != 200 {
			http.Error(w, "image unavailable", 502)
			return
		}
		raw, e = io.ReadAll(io.LimitReader(response.Body, maxImage+1))
		if e != nil || len(raw) == 0 || len(raw) > maxImage {
			http.Error(w, "image too large or unavailable", 502)
			return
		}
	}
	contentType := http.DetectContentType(raw)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		http.Error(w, "invalid image response", 502)
		return
	}
	if a.Deps.Redis != nil {
		small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		a.Deps.Redis.Set(small, key, raw, 24*time.Hour)
		stop()
	}
	tag := fmt.Sprintf(`"%x"`, sha256.Sum256(raw))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(304)
		return
	}
	if _, checkedErr := w.Write(raw); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/tmdb_image.go:tmdbImage")
		return
	}
}
