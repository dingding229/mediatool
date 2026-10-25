package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/system"
	"media-tool/internal/platform/scheduling"
)

func (a *API) operations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	out := system.Inspect(ctx, a.Deps.DB, a.Deps.Redis)
	coreNext, _ := scheduling.Next(scheduling.CoreReconcileCron, time.Now(), 0)
	out.Schedules = append([]system.Schedule{{ID: "core", Name: "核心协调（秒 分 时 日 月 周）", Cron: scheduling.CoreReconcileCron, Next: &coreNext, Enabled: true, Timezone: scheduling.Timezone, Link: "/logs"}}, out.Schedules...)
	if a.Deps.DB != nil && out.PostgreSQL.State == "ready" {
		if policy, err := a.maintenance().Policy(ctx); err == nil {
			next := policy.NextBackup
			nextSample, _ := scheduling.Next(maintenance.SampleCron, time.Now(), time.Minute)
			out.Schedules = append(out.Schedules, system.Schedule{ID: "ops-backup", Name: "系统备份", Cron: policy.Value.BackupCron, Enabled: policy.Value.BackupEnabled, Next: next, Timezone: scheduling.Timezone, Link: "/maintenance"})
			out.Schedules = append(out.Schedules, system.Schedule{ID: "ops-sample", Name: "运维采样/告警/保留清理", Cron: maintenance.SampleCron, Next: &nextSample, Enabled: true, Timezone: scheduling.Timezone, Link: "/maintenance"})
		} else {
			out.Unavailable = append(out.Unavailable, "maintenanceSchedules")
		}

		// V1 schedules are archived; V2 service schedules are introduced in F2.

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
