package sitemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/scheduling"
	"media-tool/internal/platform/trace"
	"time"
)

var ErrScheduleChanged = errors.New("schedule revision changed")
var ErrInvalidSchedule = errors.New("请输入有效 cron，刷新不得快于每15分钟")

type ScheduleConfig struct {
	Enabled          bool       `json:"enabled"`
	IntervalMinutes  int        `json:"-"`
	Cron             string     `json:"cron"`
	Timezone         string     `json:"timezone"`
	Revision         int64      `json:"revision"`
	NextRunAt        *time.Time `json:"nextRunAt"`
	LastDispatchedAt *time.Time `json:"lastDispatchedAt"`
}

func (e Engine) GetSchedule(ctx context.Context) (ScheduleConfig, error) {
	var s ScheduleConfig
	err := e.DB.QueryRow(ctx, `SELECT enabled,cron_expression,revision,next_run_at,last_dispatched_at FROM site_metric_schedule WHERE singleton`).Scan(&s.Enabled, &s.Cron, &s.Revision, &s.NextRunAt, &s.LastDispatchedAt)
	s.Timezone = scheduling.Timezone
	return s, err
}
func (e Engine) SaveSchedule(ctx context.Context, s ScheduleConfig) (ScheduleConfig, error) {
	next, err := scheduling.Next(s.Cron, time.Now(), 15*time.Minute)
	if err != nil {
		return s, ErrInvalidSchedule
	}
	tag, err := e.DB.Exec(ctx, `UPDATE site_metric_schedule SET enabled=$1,cron_expression=$2,revision=revision+1,next_run_at=$4 WHERE singleton AND revision=$3`, s.Enabled, s.Cron, s.Revision, next)
	if err != nil {
		return s, err
	}
	if tag.RowsAffected() != 1 {
		return s, ErrScheduleChanged
	}
	trace.Info(ctx, "sites", "站点定时刷新设置已更新", "enabled", s.Enabled, "cron", s.Cron)
	return e.GetSchedule(ctx)
}
func (e Engine) Schedule(ctx context.Context) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var s ScheduleConfig
	err = tx.QueryRow(ctx, `SELECT cron_expression,revision FROM site_metric_schedule WHERE singleton AND enabled AND next_run_at<=now() FOR UPDATE SKIP LOCKED`).Scan(&s.Cron, &s.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM integrations i WHERE kind='ptsite' AND enabled AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.kind='site.metrics.refresh' AND j.state IN ('pending','running') AND j.payload->>'accountId'=i.id::text) ORDER BY id`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	for _, id := range ids {
		jobID := jobs.UUID()
		payload, _ := json.Marshal(map[string]any{"accountId": id, "scheduleRevision": s.Revision})
		if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,max_attempts) VALUES($1,'site.metrics.refresh',$2,1)`, jobID, payload); err != nil {
			return err
		}
		event, _ := json.Marshal(map[string]string{"kind": "site.metrics.refresh"})
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload) VALUES($1,'job.queued',$2,$3)`, jobs.UUID(), jobID, event); err != nil {
			return err
		}
	}
	next, err := scheduling.NextOptional(s.Cron, time.Now(), 15*time.Minute)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE site_metric_schedule SET next_run_at=$1,last_dispatched_at=now() WHERE singleton`, next); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err == nil {
		trace.Info(ctx, "sites", "站点定时刷新任务已入队", "count", len(ids))
	}
	return err
}
func (e Engine) Process(ctx context.Context, j jobs.Job) error {
	var p struct {
		AccountID string `json:"accountId"`
		Revision  int64  `json:"scheduleRevision"`
	}
	if json.Unmarshal(j.Payload, &p) != nil || p.AccountID == "" {
		return jobs.ErrOperatorRequired
	}
	s, err := e.GetSchedule(ctx)
	if err != nil {
		return err
	}
	if !s.Enabled || s.Revision != p.Revision {
		trace.Info(ctx, "sites", "跳过已停用或变更的定时刷新任务", "accountId", p.AccountID)
		return nil
	}
	err = e.Collect(ctx, p.AccountID)
	if errors.Is(err, ErrBusy) || errors.Is(err, ErrChanged) {
		return nil
	}
	return err
}
