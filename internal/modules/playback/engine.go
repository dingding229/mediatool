// Package playback owns the common STRM/Emby delivery decision. A provider URL
// stays transient and is never returned in management diagnostics or logs.
package playback

import (
	"context"
	"crypto/tls"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/httpjson"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

type Options struct {
	Policy Policy
	// Deprecated: ignored. All accepted delivery is strict 302, never relay.
	ProxyFallback, ForceProxy bool
	Client                    *http.Client
	ResolveTimeout            time.Duration
}
type Engine struct {
	Resolver     storage.PlaybackResolver
	options      Options
	client       *http.Client
	directChecks directCheckCache
}
type Inspection struct {
	DirectURLReturned bool       `json:"directUrlReturned"`
	Mode              string     `json:"mode"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	RequiresUserAgent bool       `json:"requiresUserAgent"`
	RequiresHeaders   bool       `json:"requiresHeaders"`
	DeliveryReason    string     `json:"deliveryReason,omitempty"`
	RequiredUserAgent string     `json:"requiredUserAgent,omitempty"`
	TargetValidated   bool       `json:"targetValidated"`
	MediaBytesChecked bool       `json:"mediaBytesChecked"`
}

func New(resolver storage.PlaybackResolver, options Options) *Engine {
	client := options.Client
	if client == nil {
		client = &http.Client{Transport: &http.Transport{
			Proxy: nil, DialContext: options.Policy.DialContext, ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableCompression: true,
			MaxIdleConns: 50, MaxIdleConnsPerHost: 10, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10,
		}}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("too many playback redirects")
		}
		if err := options.Policy.Check(req.Context(), req.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 && req.URL.Host != via[len(via)-1].URL.Host {
			for key := range req.Header {
				if !strings.EqualFold(key, "User-Agent") && !strings.EqualFold(key, "Range") && !strings.EqualFold(key, "If-Range") {
					req.Header.Del(key)
				}
			}
		}
		return nil
	}
	return &Engine{Resolver: resolver, options: options, client: &copyClient, directChecks: directCheckCache{slots: make(chan struct{}, 4)}}
}
func (e *Engine) ResolveBudget() time.Duration {
	return requestbudget.EffectiveResolve(e.options.ResolveTimeout)
}
func (e *Engine) Inspect(ctx context.Context, ref storage.FileRef, userAgent string) (Inspection, error) {
	ref.UserAgent = userAgent
	link, err := e.resolve(ctx, ref)
	if err != nil {
		return Inspection{}, err
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	started := time.Now()
	err = e.options.Policy.Check(check, link.URL)
	cancel()
	storage.ObserveResolve(ctx, storage.StageTargetPolicy, started, err)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{DirectURLReturned: true, Mode: e.mode(link, userAgent), RequiresUserAgent: link.UserAgent != "" && link.UserAgent != userAgent, RequiresHeaders: len(link.Headers) > 0, TargetValidated: true}
	result.DeliveryReason = e.deliveryReason(link, userAgent)
	if link.UserAgent != "" && len(link.UserAgent) <= 512 && !strings.ContainsAny(link.UserAgent, "\r\n\x00") {
		result.RequiredUserAgent = link.UserAgent
	}
	if !link.ExpiresAt.IsZero() {
		result.ExpiresAt = &link.ExpiresAt
	}
	// DNS/URL validation is not proof that CDN bytes are readable.
	return result, nil
}
func (e *Engine) resolve(ctx context.Context, ref storage.FileRef) (link storage.DownloadLink, err error) {
	started := time.Now()
	defer func() { storage.ObserveResolve(ctx, storage.StageResolve, started, err) }()
	if e.Resolver == nil {
		return storage.DownloadLink{}, storage.ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, e.ResolveBudget())
	defer cancel()
	link, err = e.Resolver.Resolve(bounded, ref)
	if err != nil {
		return link, storage.ContextFailure(bounded, err)
	}
	if (!link.ExpiresAt.IsZero() && time.Until(link.ExpiresAt) < 5*time.Second) || (!link.CacheUntil.IsZero() && time.Until(link.CacheUntil) < 5*time.Second) {
		return link, storage.ErrInvalidResponse
	}
	for k, v := range link.Headers {
		if !httpguts.ValidHeaderFieldName(k) || !httpguts.ValidHeaderFieldValue(v) || forbiddenProviderHeader(http.CanonicalHeaderKey(k)) {
			return link, storage.ErrInvalidResponse
		}
	}
	return link, nil
}
func (e *Engine) mode(link storage.DownloadLink, ua string) string {
	if e.options.ForceProxy {
		return "unsupported"
	}
	u, _ := url.Parse(link.URL)
	needProxy := len(link.Headers) > 0 || (link.UserAgent != "" && link.UserAgent != ua) || (u != nil && u.Scheme != "https")
	if !needProxy {
		return "redirect"
	}

	return "unsupported"
}
func (e *Engine) Serve(w http.ResponseWriter, r *http.Request, ref storage.FileRef) {
	e.ServeRedirect(w, r, ref)
}

// ServeRedirect is a per-request delivery policy, not a claim about codecs.
// Never mutate shared engine options when a live connection setting changes.
func (e *Engine) ServeRedirect(w http.ResponseWriter, r *http.Request, ref storage.FileRef) {
	w.Header().Set("X-Media-Playback-Policy", "direct-only")
	e.serve(w, r, ref)
}
func (e *Engine) serve(w http.ResponseWriter, r *http.Request, ref storage.FileRef) {
	if r.Method != "GET" && r.Method != "HEAD" {
		writeError(w, 405, "method_not_allowed", "播放仅支持GET/HEAD")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	ref.UserAgent = r.UserAgent()
	host, _, splitErr := net.SplitHostPort(r.RemoteAddr)
	if splitErr == nil {
		ref.ClientIP = host
	}
	link, err := e.resolve(r.Context(), ref)
	if err != nil {
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			panic(http.ErrAbortHandler)
		}
		WriteError(w, err)
		return
	}
	check, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	started := time.Now()
	err = e.options.Policy.Check(check, link.URL)
	cancel()
	storage.ObserveResolve(r.Context(), storage.StageTargetPolicy, started, err)
	if err != nil {
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			panic(http.ErrAbortHandler)
		}
		WriteError(w, err)
		return
	}
	mode := e.mode(link, r.UserAgent())
	if mode != "redirect" {
		writeError(w, 409, "direct_redirect_required", "仅直链播放：当前来源需要代理或特殊请求头，禁止服务器中转")
		return
	}
	w.Header().Set("X-Media-Delivery-Reason", e.deliveryReason(link, r.UserAgent()))
	switch mode {
	case "redirect":
		if link.DirectOnly {
			if !e.checkDirect(r.Context(), link, r.UserAgent(), ref.ClientIP) {
				if errors.Is(r.Context().Err(), context.Canceled) {
					panic(http.ErrAbortHandler)
				}
				w.Header().Set("X-Media-Delivery", "unavailable")
				w.Header().Set("X-Media-Delivery-Reason", "direct_validation_failed")
				writeError(w, 502, "official_direct_validation_failed", "115官方直链未通过当前播放器UA的CDN读取验证；未回退服务器中转")
				return
			}
			w.Header().Set("X-Media-Direct-Verified", "true")
		}
		w.Header().Set("X-Media-Delivery", "redirect")
		http.Redirect(w, r, link.URL, http.StatusFound)
	}
}
func WriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, storage.ErrRequestTimeout):
		writeError(w, 504, "cloud_resolution_timeout", "云端取链超时；未回退服务器中转，请重新尝试播放或核对取链阶段日志")
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, 404, "file_not_found", "云端文件不存在")
	case errors.Is(err, storage.ErrDirectory):
		writeError(w, 400, "not_media_file", "请选择媒体文件，而不是目录")
	case errors.Is(err, storage.ErrUnauthenticated):
		writeError(w, 502, "cloud_authentication_failed", "CloudDrive2 凭据无效或过期，请更新接入凭据")
	case errors.Is(err, storage.ErrForbidden):
		writeError(w, 502, "cloud_permission_denied", "CloudDrive2 令牌没有读取该路径的权限")
	case errors.Is(err, storage.ErrNotReady):
		writeError(w, 503, "cloud_not_ready", "CloudDrive2 尚未登录或未就绪")
	case errors.Is(err, storage.ErrUnsupported):
		writeError(w, 502, "cloud_rpc_unsupported", "部署的 CloudDrive2 版本不支持此 RPC")
	case errors.Is(err, storage.ErrNoDirectURL):
		writeError(w, 502, "cloud_direct_url_unavailable", "CloudDrive2 未返回 115 直链；请检查云盘与直链能力")
	case errors.Is(err, storage.ErrNotImplemented):
		writeError(w, 501, "storage_not_implemented", "该存储操作尚未实现")
	case errors.Is(err, ErrUnsafeTarget):
		writeError(w, 502, "unsafe_redirect", "播放目标不安全、域名不在允许范围或 DNS 不可用")
	default:
		writeError(w, 502, "storage_unavailable", "无法获取云端播放地址，请检查服务与文件状态")
	}
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	if checkedErr := httpjson.Write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}}); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/modules/playback/engine.go:writeError")
		return
	}
}

// NativeClientHeaders advertises only the non-secret UA needed by the CDN.
// It never returns the temporary URL or provider cookies/authorization headers.
// Browser clients are deliberately excluded by the gateway: they cannot set UA.
func (e *Engine) NativeClientHeaders(ctx context.Context, ref storage.FileRef) map[string]string {
	if e.options.ForceProxy {
		return nil
	}
	link, err := e.resolve(ctx, ref)
	if err != nil || link.CallerUserAgent || link.UserAgent == "" || len(link.Headers) != 0 || strings.ContainsAny(link.UserAgent, "\r\n\x00") {
		return nil
	}
	u, err := url.Parse(link.URL)
	if err != nil || u.Scheme != "https" || e.options.Policy.Check(ctx, link.URL) != nil {
		return nil
	}
	return map[string]string{"User-Agent": link.UserAgent}
}

func (e *Engine) deliveryReason(link storage.DownloadLink, ua string) string {
	if e.options.ForceProxy {
		return "force_proxy_unsupported"
	}
	if len(link.Headers) > 0 {
		return "provider_headers_required"
	}
	if link.UserAgent != "" && link.UserAgent != ua {
		return "provider_user_agent_required"
	}
	if u, err := url.Parse(link.URL); err != nil || u.Scheme != "https" {
		return "non_https_source"
	}
	return "direct_conditions_met"
}

func forbiddenProviderHeader(key string) bool {
	switch key {
	case "Host", "Connection", "Content-Length", "Transfer-Encoding", "Upgrade", "Trailer", "Te", "Proxy-Authorization", "Proxy-Connection", "Range", "If-Range":
		return true
	}
	return strings.HasPrefix(key, "X-Forwarded-") || strings.HasPrefix(key, "Proxy-")
}
