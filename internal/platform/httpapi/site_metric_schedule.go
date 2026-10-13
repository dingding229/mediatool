package httpapi

import (
	"errors"
	"media-tool/internal/modules/sitemetrics"
	"net/http"
)

func (a *API) getSiteMetricSchedule(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	s, err := (sitemetrics.Engine{DB: a.Deps.DB}).GetSchedule(r.Context())
	if err != nil {
		fail(w, 503, "schedule_unavailable", "定时刷新设置不可用")
		return
	}
	write(w, 200, s)
}
func (a *API) saveSiteMetricSchedule(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var s sitemetrics.ScheduleConfig
	if !decode(w, r, &s) {
		return
	}
	s, err := (sitemetrics.Engine{DB: a.Deps.DB}).SaveSchedule(r.Context(), s)
	if errors.Is(err, sitemetrics.ErrScheduleChanged) {
		fail(w, 409, "schedule_changed", "设置已更新，请刷新重试")
		return
	}
	if errors.Is(err, sitemetrics.ErrInvalidSchedule) {
		fail(w, 400, "invalid_schedule", "刷新间隔需15分钟～7天")
		return
	}
	if err != nil {
		fail(w, 503, "schedule_unavailable", "定时刷新保存失败")
		return
	}
	write(w, 200, s)
}
