package httpapi

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	deferutil "media-tool/internal/platform/cleanup"
)

const (
	tmdbImageConcurrent = 8
	tmdbImagePerSource  = 60
	tmdbImageMaxSources = 4096
	tmdbImageMaxBytes   = 4 << 20
)

var tmdbImageFile = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}\.(?:jpg|png|webp)$`)
var errTMDBImage = errors.New("image unavailable or invalid")

type tmdbImageAdmission struct {
	mu      sync.Mutex
	active  int
	minute  int64
	sources map[string]int
	flight  singleflight.Group
}

// The global slots cover Redis, upstream work, waiters and response bodies.
// Reject rather than letting requests pile up behind the shared fetches.
func (b *tmdbImageAdmission) enter(source string, now time.Time) (func(), string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	minute := now.Unix() / 60
	if b.sources == nil || b.minute != minute {
		b.sources = make(map[string]int)
		b.minute = minute
	}
	used, known := b.sources[source]
	if used >= tmdbImagePerSource || !known && len(b.sources) >= tmdbImageMaxSources {
		return nil, "60"
	}
	if b.active >= tmdbImageConcurrent {
		return nil, "1"
	}
	b.active++
	b.sources[source] = used + 1
	return func() {
		b.mu.Lock()
		b.active--
		b.mu.Unlock()
	}, ""
}

type tmdbThumbnail struct {
	body              []byte
	contentType, etag string
}

func sniffTMDBThumbnail(raw []byte) (tmdbThumbnail, bool) {
	if len(raw) == 0 || len(raw) > tmdbImageMaxBytes {
		return tmdbThumbnail{}, false
	}
	typ := http.DetectContentType(raw)
	if typ != "image/jpeg" && typ != "image/png" && typ != "image/webp" {
		return tmdbThumbnail{}, false
	}
	return tmdbThumbnail{body: raw, contentType: typ, etag: fmt.Sprintf(`"%x"`, sha256.Sum256(raw))}, true
}

func (a *API) tmdbImage(w http.ResponseWriter, r *http.Request) {
	size, file := r.PathValue("size"), r.PathValue("file")
	if !tmdbImageFile.MatchString(file) || (size != "w185" && size != "w342" && size != "w500" && size != "w1280") {
		http.NotFound(w, r)
		return
	}
	leave, retry := a.tmdbImages.enter(requestAdmissionIP(r), time.Now())
	if leave == nil {
		w.Header().Set("Retry-After", retry)
		fail(w, 429, "tmdb_image_rate_limited", "缩略图请求过于频繁，请稍后重试")
		return
	}
	defer leave()
	result := <-a.thumbnailFlight(r.Context(), size, file)
	if r.Context().Err() != nil {
		return
	}
	if result.Err != nil {
		http.Error(w, "image unavailable or invalid", http.StatusBadGateway)
		return
	}
	image := result.Val.(tmdbThumbnail)
	w.Header().Set("Content-Type", image.contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", image.etag)
	if r.Header.Get("If-None-Match") == image.etag {
		w.WriteHeader(304)
		return
	}
	if _, checkedErr := w.Write(image.body); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/tmdb_image.go:tmdbImage")
	}
}

func (a *API) thumbnailFlight(ctx context.Context, size, file string) <-chan singleflight.Result {
	key := fmt.Sprintf("media-tool:tmdb-image-cache:%x", sha256.Sum256([]byte(size+"/"+file)))
	return a.tmdbImages.flight.DoChan(key, func() (any, error) {
		// This fetch belongs to all waiters. One disconnected client must not
		// cancel others, and each still owns its slot until bounded work ends.
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return a.loadTMDBThumbnail(bounded, key, size, file)
	})
}

func (a *API) loadTMDBThumbnail(ctx context.Context, key, size, file string) (tmdbThumbnail, error) {
	if a.Deps.Redis != nil {
		small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		length, err := a.Deps.Redis.StrLen(small, key).Result()
		if err == nil && length > 0 && length <= tmdbImageMaxBytes {
			raw, err := a.Deps.Redis.GetRange(small, key, 0, tmdbImageMaxBytes).Bytes()
			if image, ok := sniffTMDBThumbnail(raw); err == nil && ok {
				stop()
				return image, nil // Do not extend a valid entry's original TTL.
			}
		}
		stop()
	}
	client := http.Client{}
	if a.Deps.Integrations != nil {
		selected, err := a.Deps.Integrations.HTTPClient(ctx, "tmdb")
		if err != nil {
			return tmdbThumbnail{}, errTMDBImage
		}
		client = *selected
	}
	client.Timeout = 8 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Jar = nil
	// Fixed public image origin only; never attach tokens, PT cookies, or client headers.
	request, err := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/"+size+"/"+file, nil)
	if err != nil {
		return tmdbThumbnail{}, errTMDBImage
	}
	response, err := client.Do(request)
	if err != nil {
		return tmdbThumbnail{}, errTMDBImage
	}
	defer deferutil.Close(response.Body, "Bounded thumbnail response descriptor")
	if response.StatusCode != 200 {
		return tmdbThumbnail{}, errTMDBImage
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, tmdbImageMaxBytes+1))
	image, ok := sniffTMDBThumbnail(raw)
	if err != nil || !ok {
		return tmdbThumbnail{}, errTMDBImage
	}
	if a.Deps.Redis != nil {
		small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		if err := a.Deps.Redis.Set(small, key, raw, 24*time.Hour).Err(); err != nil {
			deferutil.Report(err, "optional thumbnail cache population")
		}
		stop()
	}
	return image, nil
}
