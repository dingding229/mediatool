package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"io"
	"media-tool/internal/modules/maintenance"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"strings"
	"time"
)

func (a *API) pluginExtensionStore() (pluginv2.Store, error) {
	value, e := a.pluginInstances()
	if e != nil {
		return pluginv2.Store{}, e
	}
	s, ok := value.(pluginv2.Store)
	if !ok || s.DB == nil {
		return s, pluginv2.ErrUnavailable
	}
	return s, nil
}
func (a *API) pluginGateway(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	deadline, _ := ctx.Deadline()
	if e := controller.SetReadDeadline(deadline); e != nil && !errors.Is(e, http.ErrNotSupported) {
		pluginV2Error(w, e)
		return
	}
	if e := controller.SetWriteDeadline(deadline.Add(time.Second)); e != nil && !errors.Is(e, http.ErrNotSupported) {
		pluginV2Error(w, e)
		return
	}
	if len(r.URL.RawQuery) > 4096 || len(r.URL.Path) > 512 {
		fail(w, 400, "invalid_plugin_request", "插件请求超限")
		return
	}
	id, path := r.PathValue("instance"), "/"+r.PathValue("path")
	if !contracts.ValidAPIPath(path) {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	snap, d, e := s.Descriptor(ctx, id, "api", path)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	headersSize := 0
	for k, values := range r.Header {
		headersSize += len(k)
		for _, v := range values {
			headersSize += len(v)
		}
	}
	if headersSize > 16<<10 {
		fail(w, 431, "plugin_headers_too_large", "插件请求头超限")
		return
	}
	if e = s.LimitAPI(ctx, id); e != nil {
		if errors.Is(e, pluginv2.ErrRateLimited) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "plugin_rate_limited", "插件接口每分钟最多60次")
		} else {
			pluginV2Error(w, e)
		}
		return
	}
	call := func(w http.ResponseWriter, r *http.Request) {
		allowed := false
		for _, method := range d.Methods {
			if method == r.Method {
				allowed = true
			}
		}
		if !allowed {
			fail(w, 405, "plugin_method_not_allowed", "插件未声明此方法")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && d.Auth != "admin" {
			paused, e := maintenance.Paused(r.Context(), s.DB)
			if e != nil {
				pluginV2Error(w, e)
				return
			}
			if paused {
				fail(w, 423, "maintenance_paused", "维护期间暂停插件写入")
				return
			}
		}
		a.dispatchPluginAPI(w, r, s, d, path)
	}

	switch d.Auth {
	case "admin":
		a.protected(call)(w, r)
	case "apikey", "bearer":
		token := ""
		if d.Auth == "apikey" {
			if len(r.Header.Values("X-API-Key")) != 1 || r.Header.Get("Authorization") != "" {
				fail(w, 401, "plugin_auth_required", "需要插件专属API凭据")
				return
			}
			token = r.Header.Get("X-API-Key")
		} else {
			fields := strings.Fields(r.Header.Get("Authorization"))
			if len(r.Header.Values("Authorization")) != 1 || len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || r.Header.Get("X-API-Key") != "" {
				fail(w, 401, "plugin_auth_required", "需要插件Bearer凭据")
				return
			}
			token = fields[1]
		}
		if e = s.CheckAPIKey(ctx, id, d.ID, r.Method, token, snap); e != nil {
			if errors.Is(e, pluginv2.ErrAuthentication) {
				fail(w, 401, "plugin_auth_required", "插件凭据无效、已过期或已撤销")
			} else {
				pluginV2Error(w, e)
			}
			return
		}
		call(w, r)
	case "public":
		call(w, r) // Descriptor+live grant validation above includes api.public[/write].
	default:
		pluginV2Error(w, pluginv2.ErrInvalid)
	}
}
func (a *API) dispatchPluginAPI(w http.ResponseWriter, r *http.Request, s pluginv2.Store, d contracts.Descriptor, path string) {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if e != nil {
		var size *http.MaxBytesError
		if errors.As(e, &size) {
			fail(w, 413, "plugin_body_too_large", "插件请求体超限")
		} else {
			fail(w, 503, "plugin_request_timeout", "插件请求读取超时")
		}
		return
	}
	query := r.URL.Query()
	if len(query) > 32 {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	for key := range query {
		switch strings.ToLower(key) {
		case "api_key", "apikey", "token", "access_token":
			fail(w, 400, "plugin_auth_header_only", "插件凭据只能放在请求头")
			return
		}
	}
	headers := map[string]string{}
	for _, key := range []string{"Content-Type", "Accept", "Accept-Language"} {
		if value := r.Header.Get(key); value != "" {
			headers[key] = value
		}
	}
	args, e := json.Marshal(contracts.APIRequest{Method: r.Method, Path: path, Query: query, Headers: headers, Body: body})
	if e != nil {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	operationKey := r.Header.Get("Idempotency-Key")
	if len(operationKey) > 100 || len(r.Header.Values("Idempotency-Key")) > 1 {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	out, e := s.Invoke(r.Context(), r.PathValue("instance"), "api", d.ID, args, operationKey)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	var response contracts.APIResponse
	if contracts.Decode(out, &response) != nil || validatePluginResponse(response) != nil {
		fail(w, 502, "invalid_plugin_response", "插件响应不符合白名单")
		return
	}
	// The credential and cookie never enter the worker. Plugin headers cannot set
	// cookies, redirects, CORS, framing or the application's security headers.
	w.Header().Set("Content-Type", "application/json") // No content sniffing when the plugin omits a type.
	w.Header().Set("Cache-Control", "no-store")
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(response.Status)
	if _, e = w.Write(response.Body); e != nil {
		return
	}
}
func validatePluginResponse(v contracts.APIResponse) error {
	allowed := map[int]bool{200: true, 201: true, 202: true, 204: true, 400: true, 404: true, 409: true, 422: true, 429: true, 500: true, 503: true}
	if !allowed[v.Status] || len(v.Body) > 64<<10 || len(v.Headers) > 8 || v.Status == 204 && len(v.Body) > 0 {
		return pluginv2.ErrInvalid
	}
	seen := map[string]bool{}
	for k, value := range v.Headers {
		canonical := http.CanonicalHeaderKey(k)
		if seen[canonical] {
			return pluginv2.ErrInvalid
		}
		seen[canonical] = true
		if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
			return pluginv2.ErrInvalid
		}
		switch http.CanonicalHeaderKey(k) {
		case "Content-Type":
			if value != "application/json" && value != "text/plain; charset=utf-8" {
				return pluginv2.ErrInvalid
			}
		case "Retry-After":
			if value != "1" && value != "5" && value != "30" && value != "60" {
				return pluginv2.ErrInvalid
			}
		default:
			return pluginv2.ErrInvalid
		}
	}
	return nil
}
