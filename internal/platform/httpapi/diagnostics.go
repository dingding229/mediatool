package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"strings"
	"time"

	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

func (a *API) testStorage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Path == "" {
		body.Path = a.builder(r.Context()).AllowedRoots[0]
	}
	if err := a.builder(r.Context()).Validate(storage.FileRef{Path: body.Path}); err != nil {
		fail(w, 400, "invalid_file_ref", err.Error())
		return
	}
	if !a.storageConfigured(r.Context()) {
		fail(w, 503, "storage_not_configured", "请先配置 CloudDrive2 地址及读取凭据")
		return
	}
	inspector, ok := a.Deps.Storage.(storage.Inspector)
	if !ok {
		fail(w, 501, "inspection_not_implemented", "该存储适配器没有诊断接口")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := inspector.Probe(ctx, body.Path)
	if err != nil {
		playback.WriteError(w, err)
		return
	}
	write(w, 200, result)
}
func (a *API) diagnoseStorage(w http.ResponseWriter, r *http.Request) {
	var ref storage.FileRef
	if !decode(w, r, &ref) {
		return
	}
	if err := a.builder(r.Context()).Validate(ref); err != nil {
		fail(w, 400, "invalid_file_ref", err.Error())
		return
	}
	if !a.storageConfigured(r.Context()) {
		fail(w, 503, "storage_not_configured", "请先配置 CloudDrive2 地址及读取凭据")
		return
	}
	inspector, ok := a.Deps.Storage.(storage.Inspector)
	if !ok {
		fail(w, 501, "inspection_not_implemented", "该存储适配器没有诊断接口")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(requestbudget.InspectWrite(a.Engine.ResolveBudget())))
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	file, err := inspector.Find(ctx, ref.Path)
	cancel()
	if err != nil {
		playback.WriteError(w, err)
		return
	}
	inspection, err := a.Engine.Inspect(r.Context(), ref, r.UserAgent())
	if err != nil {
		playback.WriteError(w, err)
		return
	}
	write(w, 200, map[string]any{"file": file, "playback": inspection, "note": "只验证文件、直链与地址策略；未读取视频字节，未确认真实播放器播放"})
}
func cloudMessage(err error) string {
	switch {
	case errors.Is(err, storage.ErrUnauthenticated):
		return "CloudDrive2 凭据无效或过期"
	case errors.Is(err, storage.ErrForbidden):
		return "CloudDrive2 凭据没有配置目录的读取权限"
	case errors.Is(err, storage.ErrNotFound):
		return "配置的 CloudDrive2 根目录不存在"
	case errors.Is(err, storage.ErrNotReady):
		return "CloudDrive2 尚未登录或未就绪"
	case errors.Is(err, storage.ErrUnsupported):
		return "CloudDrive2 版本缺少所需 RPC"
	default:
		return "CloudDrive2 不可达或响应无效，请检查原生 gRPC 地址与 TLS 配置"
	}
}
func (a *API) testEmby(w http.ResponseWriter, r *http.Request) {
	upstream := a.Config.EmbyUpstream
	if a.Deps.Integrations != nil {
		if c, err := a.Deps.Integrations.Repository.Default(r.Context(), "emby"); err == nil {
			upstream = c.String("endpoint")
		}
	}
	if upstream == "" {
		fail(w, 503, "emby_not_configured", "请先配置 EMBY_UPSTREAM_URL")
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	target, _ := url.Parse(upstream)
	target.Path = strings.TrimRight(target.Path, "/") + "/System/Info/Public"
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		fail(w, 502, "emby_unavailable", "Emby 地址无效")
		return
	}
	// No management cookie or API Key is forwarded. This is Emby's public info.
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		fail(w, 502, "emby_unavailable", "无法连接 Emby，请检查地址和 TLS")
		return
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if response.StatusCode != 200 {
		fail(w, 502, "emby_unavailable", "Emby 公共信息接口不可用")
		return
	}
	var info struct {
		ID      string `json:"Id"`
		Version string `json:"Version"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&info) != nil || info.ID == "" {
		fail(w, 502, "emby_invalid_response", "上游不是有效的 Emby 公共信息接口")
		return
	}
	write(w, 200, map[string]any{"reachable": true, "version": info.Version, "clientPlaybackVerified": false, "note": "此检查仅验证 Emby 服务可达，不代表用户授权或播放通过"})
}
func (a *API) checkEmbyItem(w http.ResponseWriter, r *http.Request) {
	gateway := a.gateway(r.Context())
	if gateway == nil {
		fail(w, 503, "emby_not_configured", "请先配置 EMBY_UPSTREAM_URL")
		return
	}
	var body struct {
		ItemID    string `json:"itemId"`
		SourceID  string `json:"mediaSourceId"`
		UserToken string `json:"userToken"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.UserToken == "" || strings.ContainsAny(body.UserToken, "\r\n\x00") {
		fail(w, 400, "invalid_emby_token", "需要 Emby 用户访问令牌，不能用服务管理 API Key 替代")
		return
	}
	request, _ := http.NewRequest("GET", "http://diagnostic.invalid/", nil)
	request.Header.Set("X-Emby-Token", body.UserToken)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := gateway.CheckItem(ctx, request, body.ItemID, body.SourceID)
	if err != nil {
		fail(w, 502, "emby_item_check_failed", "Emby 用户授权或媒体查询失败，请检查用户令牌和媒体 ID")
		return
	}
	write(w, 200, result)
}
