package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/platform/auth"
)

type publicOrigin struct {
	Scheme, Host, Base, Token, OriginalPath string
	NativeHeaders                           bool
	DirectPlayOnly                          bool
}
type originContextKey struct{}

var upstreamTransport = &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 32}

type ManagedPredicate func(context.Context, string) bool
type ManagedHeaders func(context.Context, string) map[string]string

func NewProxy(target *url.URL) http.Handler { return NewProxyWithManaged(target, nil) }
func NewProxyWithManaged(target *url.URL, managed ManagedPredicate, configuredOrigins ...*url.URL) http.Handler {
	return newProxy(target, managed, nil, nil, configuredOrigins...)
}

// observer sees upstream-authenticated responses before metadata rewriting.
func newProxy(target *url.URL, managed ManagedPredicate, observer func(*http.Response) error, nativeHeaders ManagedHeaders, configuredOrigins ...*url.URL) http.Handler {
	return newProxyWithTransport(target, managed, observer, nativeHeaders, defaultProxyTransport(), configuredOrigins...)
}

func newProxyWithTransport(target *url.URL, managed ManagedPredicate, observer func(*http.Response) error, nativeHeaders ManagedHeaders, transport http.RoundTripper, configuredOrigins ...*url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, "/emby")
			if r.Out.URL.Path == "" {
				r.Out.URL.Path = "/"
			}
			r.Out.URL.RawPath = ""
			r.SetURL(target)
			r.SetXForwarded()
			cookies := r.Out.Cookies()
			r.Out.Header.Del("Cookie")
			for _, cookie := range cookies {
				if cookie.Name != auth.CookieName {
					r.Out.AddCookie(cookie)
				}
			}
			r.Out.Header.Set("Accept-Encoding", "identity")
			if managed != nil && isBrowserStartupAsset(r.Out.URL.Path) {
				// A reload may present the original bundle's validators. Get bytes
				// before applying the exact-version, fail-safe startup policy.
				r.Out.Header.Del("If-None-Match")
				r.Out.Header.Del("If-Modified-Since")
			}
		},
		Transport: transport,
		ModifyResponse: func(response *http.Response) error {
			if managed != nil && response.Request.Context().Value(directPlayOnlyKey{}) != true {
				if err := rewriteBrowserStartup(response); err != nil {
					return err
				}
			}
			if observer != nil {
				if err := observer(response); err != nil {
					return err
				}
			}
			origin, _ := response.Request.Context().Value(originContextKey{}).(publicOrigin)
			origin.DirectPlayOnly = response.Request.Context().Value(directPlayOnlyKey{}) == true
			if origin.Host == "" {
				return nil
			}
			if location := response.Header.Get("Location"); location != "" {
				if u, err := url.Parse(location); err == nil && (u.Host == "" || u.Host == target.Host) {
					response.Header.Set("Location", publicPath(origin, u.Path, u.RawQuery))
				}
			}
			contentType := response.Header.Get("Content-Type")
			if response.StatusCode != 200 || !strings.Contains(contentType, "application/json") {
				return nil
			}
			p := strings.ToLower(origin.OriginalPath)
			if !strings.Contains(p, "playbackinfo") && !strings.Contains(p, "/items") {
				return nil
			}
			originalBody := response.Body
			raw, err := io.ReadAll(io.LimitReader(originalBody, (16<<20)+1))
			if err != nil {
				return err
			}
			if len(raw) > 16<<20 {
				deferutil.Close(originalBody, "Read/descriptor teardown only; primary operations handle their own I/O result")
				return errors.New("metadata exceeds safe rewrite limit")
			}
			deferutil.Close(originalBody, "Read/descriptor teardown only; primary operations handle their own I/O result")
			var value any
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if decoder.Decode(&value) != nil {
				return errors.New("invalid metadata JSON")
			}
			rewriteJSON(response.Request.Context(), value, target, origin, "", managed, nativeHeaders)
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			response.Body = io.NopCloser(bytes.NewReader(encoded))
			response.ContentLength = int64(len(encoded))
			response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
			response.Header.Del("Content-Encoding")
			response.Header.Del("ETag")
			response.Header.Set("Cache-Control", "no-store")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("X-Media-Gateway", "emby-upstream")
			reason := proxyFailureReason(r.Context(), err)
			w.Header().Set("X-Media-Gateway-Reason", reason)
			if reason == "request_cancelled" {
				// A player's cancelled fragment request is not a delivered 502.
				// Let net/http abort; do not write an error to a disconnected client.
				panic(http.ErrAbortHandler)
			}
			http.Error(w, "Emby upstream unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base := "/emby"
		if root, _ := r.Context().Value(gatewayRootKey{}).(bool); root {
			base = ""
		}
		host := r.Host
		if len(configuredOrigins) > 0 && configuredOrigins[0] != nil {
			scheme = configuredOrigins[0].Scheme
			host = configuredOrigins[0].Host
		}
		origin := publicOrigin{Scheme: scheme, Host: host, Base: base, Token: callerToken(r), OriginalPath: strings.TrimPrefix(r.URL.Path, "/emby"), NativeHeaders: supportsNativeHeaders(r) && strings.Contains(strings.ToLower(r.URL.Path), "playbackinfo")}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), originContextKey{}, origin)))
	})
}
func publicPath(origin publicOrigin, p, query string) string {
	u := url.URL{Scheme: origin.Scheme, Host: origin.Host, Path: origin.Base + "/" + strings.TrimPrefix(p, "/"), RawQuery: query}
	return u.String()
}
func callerToken(r *http.Request) string {
	if value := queryFold(r.URL.Query(), "api_key"); value != "" {
		return value
	}
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := r.Header.Get(header); value != "" {
			return value
		}
	}
	for _, header := range []string{"Authorization", "X-Emby-Authorization"} {
		value := r.Header.Get(header)
		if strings.HasPrefix(value, "Bearer ") {
			return strings.TrimPrefix(value, "Bearer ")
		}
		if match := regexp.MustCompile(`(?i)Token="([^"]+)"`).FindStringSubmatch(value); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}
func rewriteJSON(ctx context.Context, value any, target *url.URL, origin publicOrigin, itemID string, predicate ManagedPredicate, headers ManagedHeaders) {
	switch node := value.(type) {
	case map[string]any:
		if id, ok := node["Id"].(string); ok && node["MediaSources"] != nil {
			itemID = id
		}
		if itemID == "" {
			match := regexp.MustCompile(`(?i)^/Items/([A-Za-z0-9_-]+)/PlaybackInfo`).FindStringSubmatch(origin.OriginalPath)
			if len(match) == 2 {
				itemID = match[1]
			}
		}
		if sources, ok := node["MediaSources"].([]any); ok {
			for _, raw := range sources {
				source, _ := raw.(map[string]any)
				if source == nil {
					continue
				}
				managed, ok := source["Path"].(string)
				if !ok {
					continue
				}
				if predicate == nil || !predicate(ctx, managed) {
					continue
				}
				if origin.DirectPlayOnly {
					// Explicit native/direct policy: expose the source's fixed STRM
					// grant as requested, not an Emby stream surrogate or CDN URL.
					// The upstream has already authorized this metadata response.
					source["Path"] = managed
					source["IsRemote"] = true
					source["Protocol"] = "Http"
				} else if itemID != "" {
					sourceID, _ := source["Id"].(string)
					query := url.Values{"Static": {"true"}, "MediaSourceId": {sourceID}}
					if origin.Token != "" {
						query.Set("api_key", origin.Token)
					}
					link := publicPath(origin, "/Videos/"+url.PathEscape(itemID)+"/stream", query.Encode())
					source["Path"] = link
					// DirectStream may be an HLS/remux endpoint selected by Emby's
					// device profile. Replacing it with whole-file Static=true breaks
					// browsers expecting a playlist. Only replace a missing/grant URL.
					direct, _ := source["DirectStreamUrl"].(string)
					if direct == "" || predicate(ctx, direct) {
						source["DirectStreamUrl"] = "Videos/" + url.PathEscape(itemID) + "/stream?" + query.Encode()
					} else {
						source["DirectStreamUrl"] = rewriteMediaURL(direct, target, origin)
					}
					if transcoding, ok := source["TranscodingUrl"].(string); ok {
						source["TranscodingUrl"] = rewriteMediaURL(transcoding, target, origin)
					}
				} else {
					delete(source, "Path")
					delete(source, "DirectStreamUrl")
				}
				if origin.DirectPlayOnly {
					// Keep the server/device's true DirectPlay capability. Do not
					// invent TS/AC3/browser support merely to force a play button.
					source["SupportsTranscoding"] = false
					source["SupportsDirectStream"] = false
					delete(source, "TranscodingUrl")
					delete(source, "TranscodingContainer")
					delete(source, "TranscodingSubProtocol")
					// A previous remux/HLS URL must not survive strict mode.
					delete(source, "DirectStreamUrl")
				}
				delete(source, "RequiredHttpHeaders")
				if origin.NativeHeaders && !origin.DirectPlayOnly && headers != nil {
					if values := headers(ctx, managed); len(values) > 0 {
						source["RequiredHttpHeaders"] = values
						// A validated cloud file is a remote HTTP source. Make that explicit
						// for clients which apply headers only to remote sources, but do not
						// switch to the absolute Path if caller credentials were not captured.
						if origin.Token != "" {
							source["IsRemote"] = true
							source["Protocol"] = "Http"
						}
					}
				}
			}
		}
		for key, child := range node {
			if text, ok := child.(string); ok {
				if key == "DirectStreamUrl" || key == "TranscodingUrl" {
					node[key] = rewriteMediaURL(text, target, origin)
				} else if key == "Path" && origin.DirectPlayOnly && predicate != nil && predicate(ctx, text) {
					// Preserve the original fixed URL byte-for-byte, including its
					// query order/encoding, even on a same-host upstream.
					continue
				} else if key == "Path" || strings.HasSuffix(key, "Url") || strings.HasSuffix(key, "URL") {
					if u, err := url.Parse(text); err == nil && u.Host == target.Host {
						node[key] = publicPath(origin, u.Path, u.RawQuery)
					}
				}
			} else {
				rewriteJSON(ctx, child, target, origin, itemID, predicate, headers)
			}
		}
	case []any:
		for _, child := range node {
			rewriteJSON(ctx, child, target, origin, itemID, predicate, headers)
		}
	}
}

type gatewayRootKey struct{}

// IsRootOrigin distinguishes the Emby listener from the management SPA.
func IsRootOrigin(ctx context.Context) bool {
	value, _ := ctx.Value(gatewayRootKey{}).(bool)
	return value
}

func WithRootOrigin(ctx context.Context) context.Context {
	return context.WithValue(ctx, gatewayRootKey{}, true)
}

type joinedReadCloser struct {
	io.Reader
	io.Closer
}

// Keep relative and same-upstream media URLs on the gateway while preserving
// all remux/transcoding parameters and foreign CDN URLs verbatim.
func rewriteMediaURL(raw string, target *url.URL, origin publicOrigin) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return raw
	}
	if u.Host == "" && u.Scheme == "" || u.Host == target.Host {
		mediaPath := u.Path
		prefix := strings.TrimRight(target.Path, "/")
		if prefix != "" && strings.HasPrefix(mediaPath, prefix+"/") {
			mediaPath = strings.TrimPrefix(mediaPath, prefix)
		}
		// Emby clients call ApiClient.getUrl() on these fields. Absolute URLs
		// become /http:/host/... and 404; keep their server-relative contract.
		u.Scheme, u.Host = "", ""
		u.Path, u.RawPath = strings.TrimPrefix(mediaPath, "/"), ""
		return u.String()
	}
	return raw
}

var nativeClientPattern = regexp.MustCompile(`(?i)Client="SenPlayer"`)

func supportsNativeHeaders(r *http.Request) bool {
	if strings.HasPrefix(strings.ToLower(r.UserAgent()), "senplayer/") {
		return true
	}
	for _, key := range []string{"Authorization", "X-Emby-Authorization"} {
		if nativeClientPattern.MatchString(r.Header.Get(key)) {
			return true
		}
	}
	return false
}

// Only fixed diagnostic classes escape this function. Never return err.Error():
// transport errors can carry URL queries or sensitive upstream response details.
func proxyFailureReason(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "request_cancelled"
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return "upstream_timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "upstream_closed"
	}
	return "upstream_unavailable"
}
