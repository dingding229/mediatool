package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins"
	"media-tool/internal/modules/system"
	"media-tool/internal/platform/scheduling"
)

func (a *API) operations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	out := system.Inspect(ctx, a.Deps.DB, a.Deps.Redis)
	out.Schedules = append([]system.Schedule{{ID: "core", Name: "核心协调（秒 分 时 日 月 周）", Cron: scheduling.CoreReconcileCron, Enabled: true, Timezone: scheduling.Timezone, Link: "/logs"}}, out.Schedules...)
	if a.Deps.DB != nil && out.PostgreSQL.State == "ready" {
		rows, err := a.Deps.DB.Query(ctx, `SELECT p.id,p.name,p.enabled,p.manifest FROM extension_plugins p ORDER BY p.id LIMIT 100`)
		type entry struct {
			id, name string
			enabled  bool
			raw      []byte
		}
		list := []entry{}
		if err == nil {
			for rows.Next() {
				var e entry
				if err = rows.Scan(&e.id, &e.name, &e.enabled, &e.raw); err != nil {
					break
				}
				list = append(list, e)
			}
			if rows.Err() != nil {
				err = rows.Err()
			}
			rows.Close()
		}
		if err != nil {
			out.Unavailable = append(out.Unavailable, "pluginSchedules")
		} else {
			for _, e := range list {
				var m plugins.Manifest
				if json.Unmarshal(e.raw, &m) != nil {
					out.Unavailable = append(out.Unavailable, "plugin:"+e.id)
					continue
				}
				if len(m.Services) == 0 {
					continue
				}
				values, _, _, configErr := a.pluginRegistry().Settings(ctx, e.id)
				for _, service := range m.Services {
					item := system.Schedule{ID: e.id + ":" + service.ID, Name: e.name + " · " + service.Name, Cron: service.Cron, Enabled: e.enabled, Link: "/plugins", Timezone: scheduling.Timezone}
					if service.ScheduleSetting != "" {
						item.Cron = values[service.ScheduleSetting]
					}
					if configErr != nil {
						item.Error = configErr.Error()
					}
					err := a.Deps.DB.QueryRow(ctx, `SELECT next_at FROM extension_service_ticks WHERE plugin_id=$1 AND service_id=$2`, e.id, service.ID).Scan(&item.Next)
					if err != nil && !errors.Is(err, pgx.ErrNoRows) {
						item.Error = err.Error()
					}
					out.Schedules = append(out.Schedules, item)
				}
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) browseJobs(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "database_unavailable", "PostgreSQL未连接")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页参数无效")
		return
	}
	q := r.URL.Query()
	f := jobs.Filter{State: q.Get("state"), Kind: q.Get("kind"), Queue: q.Get("queue"), Query: q.Get("q"), Limit: p.Limit, Offset: p.Offset}
	if len(f.State) > 32 || len(f.Kind) > 100 || len(f.Queue) > 100 || len(f.Query) > 200 {
		fail(w, 400, "invalid_filter", "筛选参数过长")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, err := (jobs.Repository{DB: a.Deps.DB}).Browse(ctx, f)
	if err != nil {
		fail(w, 503, "jobs_unavailable", "任务分页读取失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) jobDetail(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "database_unavailable", "PostgreSQL未连接")
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_job", "任务ID无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, err := (jobs.Repository{DB: a.Deps.DB}).Detail(ctx, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "job_not_found", "任务不存在")
		return
	}
	if err != nil {
		fail(w, 503, "job_unavailable", "读取任务失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
