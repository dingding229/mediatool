package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"net/http"
	"strings"
)

type NativeProbeRetrier interface {
	Retry(context.Context, string) (bool, error)
}
type nativeProbeQueued struct {
	AssetID string `json:"assetId"`
	Queued  bool   `json:"queued"`
}

func (a *API) retryNativeProbe(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.PathValue("assetId"))
	if !isWorkbenchUUID(id) {
		fail(w, 400, "invalid_probe_asset", "请选择有效的媒体资产")
		return
	}
	service := a.Deps.NativeProbeRetry
	if service == nil {
		service = probequeue.Repository{DB: a.Deps.DB}
	}
	queued, err := service.Retry(r.Context(), id)
	if errors.Is(err, probequeue.ErrNotExposable) {
		fail(w, 404, "probe_asset_not_found", "未找到可探测的媒体资产")
		return
	}
	if errors.Is(err, probequeue.ErrDisabled) {
		fail(w, 503, "native_probe_disabled", "请先启用内置媒体库和媒体探测")
		return
	}
	if err != nil {
		fail(w, 503, "native_probe_unavailable", "探测任务暂不可用")
		return
	}
	write(w, 202, nativeProbeQueued{AssetID: id, Queued: queued})
}
