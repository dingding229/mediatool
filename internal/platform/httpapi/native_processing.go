package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"net/http"
	"strconv"
	"time"
)

type NativeProcessingService interface {
	Summary(context.Context) (admin.Summary, error)
	Settings(context.Context, admin.Settings) (admin.Settings, error)
	List(context.Context, admin.Filter) (admin.Page, error)
	Retry(context.Context, admin.Retry) (admin.Retried, error)
	Audit(context.Context, string, any) error
}
type NativeCollector interface {
	Collect(context.Context) (stillstore.GCReport, error)
}
type nativeGC struct {
	Scanned        int   `json:"scanned"`
	Reclaimed      int   `json:"reclaimed"`
	ReclaimedBytes int64 `json:"reclaimedBytes"`
	Done           bool  `json:"done"`
	Partial        bool  `json:"partial"`
}

func (a *API) processing() (NativeProcessingService, error) {
	if a.Deps.NativeProcessing != nil {
		return a.Deps.NativeProcessing, nil
	}
	r, err := stillqueue.New(a.Deps.DB, stillqueue.Config{ResolveTimeout: a.Config.PlaybackResolveTimeout, FrameTimeout: a.Config.NativeStillsFrameTimeout, MaxBytes: a.Config.NativeStillsMaxBytes})
	if err != nil {
		return nil, errors.New("processing service unavailable")
	}
	return admin.Repository{DB: a.Deps.DB, Stills: r, MaxBytes: r.Limits().MaxBytes}, nil
}
func processingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, admin.ErrInvalid), errors.Is(err, probequeue.ErrInvalid), errors.Is(err, stillqueue.ErrInvalid):
		fail(w, 400, "invalid_processing_request", "预处理参数无效")
	case errors.Is(err, admin.ErrConflict):
		fail(w, 409, "processing_conflict", "设置已变化，请刷新后重试")
	case errors.Is(err, probequeue.ErrDisabled), errors.Is(err, stillqueue.ErrDisabled):
		fail(w, 503, "processing_disabled", "请先启用原生服务及对应预处理开关")
	case errors.Is(err, probequeue.ErrNotExposable), errors.Is(err, stillqueue.ErrNotEligible):
		fail(w, 404, "processing_not_found", "未找到可处理的媒体")
	default:
		fail(w, 503, "processing_unavailable", "预处理管理暂不可用，请稍后重试")
	}
}
func (a *API) processingSummary(w http.ResponseWriter, r *http.Request) {
	service, err := a.processing()
	if err != nil {
		processingError(w, err)
		return
	}
	out, err := service.Summary(r.Context())
	if err != nil {
		processingError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) processingSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled       *bool `json:"enabled"`
		ProbeEnabled  *bool `json:"probeEnabled"`
		StillsEnabled *bool `json:"stillsEnabled"`
		Revision      int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil || in.ProbeEnabled == nil || in.StillsEnabled == nil || in.Revision < 1 {
		processingError(w, admin.ErrInvalid)
		return
	}
	service, err := a.processing()
	if err != nil {
		processingError(w, err)
		return
	}
	out, err := service.Settings(r.Context(), admin.Settings{Enabled: *in.Enabled, ProbeEnabled: *in.ProbeEnabled, StillsEnabled: *in.StillsEnabled, Revision: in.Revision})
	if err != nil {
		processingError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) processingTasks(w http.ResponseWriter, r *http.Request) {
	f := admin.Filter{Kind: r.URL.Query().Get("kind"), State: r.URL.Query().Get("state")}
	for _, v := range []struct {
		key string
		n   *int
	}{{"page", &f.Page}, {"pageSize", &f.PageSize}} {
		if raw, ok := r.URL.Query()[v.key]; ok {
			if len(raw) != 1 {
				processingError(w, admin.ErrInvalid)
				return
			}
			n, err := strconv.Atoi(raw[0])
			if err != nil || n < 1 {
				processingError(w, admin.ErrInvalid)
				return
			}
			*v.n = n
		}
	}
	for _, key := range []string{"kind", "state"} {
		if len(r.URL.Query()[key]) > 1 {
			processingError(w, admin.ErrInvalid)
			return
		}
	}
	if !f.Valid() {
		processingError(w, admin.ErrInvalid)
		return
	}
	service, err := a.processing()
	if err != nil {
		processingError(w, err)
		return
	}
	out, err := service.List(r.Context(), f)
	if err != nil {
		processingError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) processingRetry(w http.ResponseWriter, r *http.Request) {
	var in admin.Retry
	if !decode(w, r, &in) {
		return
	}
	f := admin.Filter{Kind: in.Kind, State: in.State}
	if !f.Valid() || in.Bulk && (!in.Confirmed || in.ID != "") || !in.Bulk && !validProcessingID(in.Kind, in.ID) {
		processingError(w, admin.ErrInvalid)
		return
	}
	service, err := a.processing()
	if err != nil {
		processingError(w, err)
		return
	}
	out, err := service.Retry(r.Context(), in)
	if err != nil {
		processingError(w, err)
		return
	}
	write(w, 202, out)
}
func validProcessingID(kind, id string) bool {
	if kind == "probe" {
		return isWorkbenchUUID(id)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return kind == "still" && err == nil && n >= 100000
}
func (a *API) processingGC(w http.ResponseWriter, r *http.Request) {
	collector, ok := a.Deps.NativeStills.(NativeCollector)
	if !ok {
		processingError(w, errors.New("collector unavailable"))
		return
	}
	service, err := a.processing()
	if err != nil {
		processingError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err = service.Audit(ctx, "native.processing.gc.request", struct{}{}); err != nil {
		processingError(w, err)
		return
	}
	report, err := collector.Collect(ctx)
	out := nativeGC{report.Visited, report.Reclaimed, report.ReclaimedBytes, report.Done, err != nil || report.Failures > 0}
	if err != nil && report.Visited == 0 {
		processingError(w, err)
		return
	}
	if err = service.Audit(ctx, "native.processing.gc.result", out); err != nil {
		processingError(w, err)
		return
	}
	write(w, 200, out)
}
