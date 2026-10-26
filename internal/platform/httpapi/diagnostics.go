package httpapi

import (
	"context"

	"errors"

	"net/http"

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
