package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/sites"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxFaviconBytes = 256 << 10

var faviconLocks sync.Map // bounded by the compiled, finite template catalog
func faviconContentType(raw []byte) string {
	if len(raw) == 0 || len(raw) > maxFaviconBytes {
		return ""
	}
	kind := http.DetectContentType(raw)
	switch kind {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/vnd.microsoft.icon":
		return kind
	}
	return ""
}
func fetchSiteFavicon(ctx context.Context, client *http.Client, source string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", source, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/png,image/x-icon,image/jpeg,image/webp,image/gif")
	req.Header.Set("User-Agent", "MediaTool-Favicon/1.0")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer deferutil.Close(response.Body, "Site favicon read completed")
	if response.StatusCode != 200 {
		return nil, errors.New("site favicon unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFaviconBytes+1))
	if err != nil || faviconContentType(raw) == "" {
		return nil, errors.New("unsupported site favicon")
	}
	return raw, nil
}
func (a *API) ptTemplateFavicon(w http.ResponseWriter, r *http.Request) {
	template, ok := sites.GoTemplate(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Only pinned catalog origins, never Host/forwarded headers, query URLs or
	// account credentials. URL policy and pinned dialing reject private targets.
	policy := playback.Policy{AllowHTTP: true}
	target, err := policy.Syntax(template.IconURL)
	if err != nil || !strings.HasSuffix(target.Path, "/favicon.ico") || target.RawQuery != "" || a.Deps.Redis == nil {
		http.NotFound(w, r)
		return
	}
	ctx, stop := context.WithTimeout(r.Context(), 8*time.Second)
	defer stop()
	lock, _ := faviconLocks.LoadOrStore(template.ID, make(chan struct{}, 1))
	gate := lock.(chan struct{})
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		http.Error(w, "favicon unavailable", 503)
		return
	}
	key := fmt.Sprintf("media-tool:site-favicon:%x", sha256.Sum256([]byte(template.IconURL)))
	raw, cacheErr := a.Deps.Redis.Get(ctx, key).Bytes()
	if cacheErr != nil {
		if a.Deps.Integrations == nil {
			http.NotFound(w, r)
			return
		}
		record, e := (networkproxy.Repository{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}).Get(ctx)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		client, e := networkproxy.PublicClient(record, "ptsite", policy)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		client.Timeout = 6 * time.Second
		client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if len(via) > 2 || next.URL.Hostname() != target.Hostname() || next.URL.User != nil || next.URL.Scheme != "https" && next.URL.Scheme != target.Scheme {
				return http.ErrUseLastResponse
			}
			next.Header.Del("Authorization")
			next.Header.Del("Cookie")
			return nil
		}
		defer client.CloseIdleConnections()
		raw, e = fetchSiteFavicon(ctx, client, template.IconURL)
		ttl := 24 * time.Hour
		if e != nil {
			raw = []byte{}
			ttl = 5 * time.Minute
		}
		if e = a.Deps.Redis.Set(ctx, key, raw, ttl).Err(); e != nil {
			deferutil.Report(e, "Site favicon cache update")
		}
	}
	kind := faviconContentType(raw)
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	tag := fmt.Sprintf(`"%x"`, sha256.Sum256(raw))
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", tag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(304)
		return
	}
	http.ServeContent(w, r, "favicon", time.Time{}, bytes.NewReader(raw))
}
