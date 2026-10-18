package networkproxy

import (
	"context"
	"errors"
	"io"
	"media-tool/internal/modules/playback"
	dbutil "media-tool/internal/platform/database"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrHostLimited = errors.New("host HTTP budget exhausted")
var ErrHostUnsupported = errors.New("host HTTP request unsupported")

type window struct {
	at   time.Time
	used int
}
type HostClient struct {
	Repository Repository
	Policy     playback.Policy
	mu         sync.Mutex
	windows    map[string]window
	slots      chan struct{}
}
type HostResult struct {
	Status      int
	ContentType string
	BytesRead   int64
}

func NewHostClient(r Repository) *HostClient {
	return &HostClient{Repository: r, Policy: playback.Policy{AllowHTTP: true}, windows: map[string]window{}, slots: make(chan struct{}, 2)}
}
func (h *HostClient) Request(ctx context.Context, instance, profile, origin, path, method string, origins []string) (out HostResult, err error) {
	if profile != "host" || method != "GET" && method != "HEAD" {
		return out, ErrHostUnsupported
	}
	u, e := url.Parse(origin)
	if e != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !slices.Contains(origins, origin) {
		return out, playback.ErrUnsafeTarget
	}
	relative, e := url.Parse(path)
	if e != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || relative.IsAbs() || relative.Host != "" || relative.Fragment != "" || strings.ContainsAny(path, "\\\r\n\x00") {
		return out, playback.ErrUnsafeTarget
	}
	target := origin + path
	if e = h.Policy.Check(ctx, target); e != nil {
		return out, e
	}
	h.mu.Lock()
	now := time.Now()
	for k, w := range h.windows {
		if now.Sub(w.at) > 5*time.Minute {
			delete(h.windows, k)
		}
	}
	w, existed := h.windows[instance]
	if now.Sub(w.at) >= time.Minute {
		w = window{at: now}
	}
	if w.used >= 30 || len(h.windows) >= 1024 && !existed {
		h.mu.Unlock()
		return out, ErrHostLimited
	}
	w.used++
	h.windows[instance] = w
	h.mu.Unlock()
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		return out, ErrHostLimited
	}
	config, e := h.Repository.Get(ctx)
	if e != nil {
		return out, e
	}
	client, e := Client(config, "plugin")
	if e != nil {
		return out, e
	}
	client.Timeout = 10 * time.Second
	// Without a proxy, enforce public DNS again at dial time (rebind-safe).
	transport := client.Transport.(*http.Transport)
	if config.URL == "" {
		transport.DialContext = h.Policy.DialContext
	}
	defer transport.CloseIdleConnections()
	if e = dbutil.CheckEffect(ctx); e != nil {
		return out, e
	}
	request, e := http.NewRequestWithContext(ctx, method, target, nil)
	if e != nil {
		return out, e
	}
	request.Header.Set("User-Agent", "MediaTool-Plugin/2")
	request.Header.Set("Accept", "application/json,text/plain,text/html")
	response, e := client.Do(request)
	if e != nil {
		return out, e
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	n, e := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if e != nil {
		return out, e
	}
	content, _, e := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if e != nil {
		content = ""
	}
	if content != "application/json" && content != "text/plain" && content != "text/html" {
		content = ""
	}
	return HostResult{Status: response.StatusCode, ContentType: content, BytesRead: n}, nil
}
