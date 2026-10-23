package httpapi

import (
	"context"
	"media-tool/internal/modules/playbackstats"
	"net/http"
	"time"
)

func statisticsFilter(w http.ResponseWriter, r *http.Request) (playbackstats.Filter, bool) {
	start, limit, ok := managementPagination(w, r)
	if !ok {
		return playbackstats.Filter{}, false
	}
	days, e := queryInt(r.URL.Query(), "days", 30, 0, 90)
	f := playbackstats.Filter{UserID: r.URL.Query().Get("userId"), DeviceID: r.URL.Query().Get("deviceId"), Days: days, Start: start, Limit: limit}
	if e != nil || f.Validate() != nil {
		fail(w, 400, "invalid_statistics_filter", "统计范围或用户/设备无效")
		return f, false
	}
	return f, true
}
func (a *API) playbackStatistics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	f, ok := statisticsFilter(w, r)
	if !ok {
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	v, e := (playbackstats.Repository{DB: a.Deps.DB}).Summary(r.Context(), f)
	if e != nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) playbackEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	f, ok := statisticsFilter(w, r)
	if !ok {
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	v, e := (playbackstats.Repository{DB: a.Deps.DB}).Events(r.Context(), f)
	if e != nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) playbackIPs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	f, ok := statisticsFilter(w, r)
	if !ok {
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	repo := playbackstats.Repository{DB: a.Deps.DB}
	group := r.URL.Query().Get("group")
	var v playbackstats.Page[playbackstats.IPRow]
	var e error
	if group == "country" || group == "province" {
		v, e = repo.GroupedIPs(r.Context(), f, func(ip string) string {
			if a.Deps.IPGeo == nil {
				return ""
			}
			l := a.Deps.IPGeo.Lookup(ip)
			if group == "country" {
				return l.Country
			}
			if l.Province == "" {
				return l.Country
			}
			return l.Country + " " + l.Province
		})
	} else if group == "" || group == "ip" {
		v, e = repo.IPs(r.Context(), f)
	} else {
		fail(w, 400, "invalid_ip_group", "归属地汇总方式无效")
		return
	}
	if e != nil {
		fail(w, 503, "statistics_unavailable", "统计暂不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) deviceIPHistory(w http.ResponseWriter, r *http.Request) {
	f, ok := statisticsFilter(w, r)
	if !ok {
		return
	}
	if f.UserID == "" || f.DeviceID == "" {
		fail(w, 400, "invalid_device_ip_filter", "请选择媒体用户和设备")
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "statistics_unavailable", "设备 IP 历史暂不可用")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	v, e := (playbackstats.Repository{DB: a.Deps.DB}).DeviceIPs(ctx, f)
	if e != nil {
		fail(w, 503, "statistics_unavailable", "设备 IP 历史暂不可用")
		return
	}
	write(w, 200, v)
}
