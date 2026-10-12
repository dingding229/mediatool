package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
	"media-tool/internal/modules/jobs"
	"strconv"
	"time"
	_ "time/tzdata"
)

func (r Registry) Schedule(ctx context.Context) error {
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var locked bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(91220863)`).Scan(&locked); e != nil || !locked {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT id,manifest,revision FROM extension_plugins WHERE enabled ORDER BY id FOR SHARE`)
	if e != nil {
		return e
	}
	type item struct {
		id       string
		m        Manifest
		revision int64
	}
	list := []item{}
	for rows.Next() {
		var x item
		var raw []byte
		if rows.Scan(&x.id, &raw, &x.revision) != nil {
			rows.Close()
			return ErrInvalid
		}
		if json.Unmarshal(raw, &x.m) != nil {
			rows.Close()
			return ErrInvalid
		}
		list = append(list, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, x := range list {
		if Validate(x.m) != nil {
			return ErrInvalid
		}
		values, _, revision, e := r.Settings(ctx, x.id)
		if e != nil || revision != x.revision {
			return ErrInvalid
		}
		for _, s := range x.m.Services {
			next, e := nextService(s, values, time.Now())
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO extension_service_ticks(plugin_id,service_id,revision,next_at)VALUES($1,$2,$3,$4)ON CONFLICT(plugin_id,service_id)DO UPDATE SET revision=excluded.revision,next_at=CASE WHEN extension_service_ticks.revision<>excluded.revision THEN excluded.next_at ELSE extension_service_ticks.next_at END`, x.id, s.ID, x.revision, next)
			if e != nil {
				return e
			}
			var due bool
			if e = tx.QueryRow(ctx, `SELECT next_at<=now() FROM extension_service_ticks WHERE plugin_id=$1 AND service_id=$2 FOR UPDATE`, x.id, s.ID).Scan(&due); e != nil {
				return e
			}
			if !due {
				continue
			}
			if e = r.queueServiceTx(ctx, tx, x.id, s.ID, x.revision); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE extension_service_ticks SET next_at=$3 WHERE plugin_id=$1 AND service_id=$2`, x.id, s.ID, next)
			if e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}
func (r Registry) queueServiceTx(ctx context.Context, tx pgx.Tx, id, service string, revision int64) error {
	var active bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM extension_jobs WHERE plugin_id=$1 AND service_id=$2 AND state IN('pending','running'))`, id, service).Scan(&active); e != nil {
		return e
	}
	if active {
		return nil
	}
	invocation, job := jobs.UUID(), jobs.UUID()
	raw, _ := json.Marshal(map[string]string{"invocationId": invocation})
	_, e := tx.Exec(ctx, `INSERT INTO extension_jobs(id,plugin_id,service_id,revision,state)VALUES($1,$2,$3,$4,'pending')`, invocation, id, service, revision)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'plugin.service',$2,$3,1)`, job, raw, "plugin.service:"+invocation)
	return e
}
func (r Registry) RunService(ctx context.Context, id, service string, revision int64) error {
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var current int64
	var enabled bool
	e = tx.QueryRow(ctx, `SELECT manifest,revision,enabled FROM extension_plugins WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &current, &enabled)
	if e != nil || !enabled || current != revision {
		return ErrInvalid
	}
	var m Manifest
	if json.Unmarshal(raw, &m) != nil {
		return ErrInvalid
	}
	found := false
	for _, s := range m.Services {
		if s.ID == service {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	if e = r.queueServiceTx(ctx, tx, id, service, revision); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (r Registry) ProcessService(ctx context.Context, job jobs.Job) error {
	var in struct {
		ID string `json:"invocationId"`
	}
	if json.Unmarshal(job.Payload, &in) != nil {
		return ErrInvalid
	}
	var id, service, state string
	var revision, current int64
	var enabled bool
	e := r.DB.QueryRow(ctx, `SELECT j.plugin_id,j.service_id,j.revision,j.state,p.revision,p.enabled FROM extension_jobs j JOIN extension_plugins p ON p.id=j.plugin_id WHERE j.id=$1`, in.ID).Scan(&id, &service, &revision, &state, &current, &enabled)
	if e != nil {
		return e
	}
	if state == "succeeded" || state == "cancelled" {
		return nil
	}
	if state != "pending" {
		return jobs.ErrOperatorRequired
	}
	if !enabled || current != revision {
		_, e = r.DB.Exec(ctx, `UPDATE extension_jobs SET state='cancelled',finished_at=now()WHERE id=$1`, in.ID)
		return e
	}
	tag, e := r.DB.Exec(ctx, `UPDATE extension_jobs SET state='running' WHERE id=$1 AND state='pending'`, in.ID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return jobs.ErrOperatorRequired
	}
	_, e = r.Invoke(ctx, id, "plugin.service", map[string]any{"service": service, "invocationId": in.ID}, false)
	end, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	state = "succeeded"
	if e != nil {
		state = "outcome_unknown"
	}
	_, save := r.DB.Exec(end, `UPDATE extension_jobs SET state=$2,finished_at=now()WHERE id=$1`, in.ID, state)
	if e != nil || save != nil {
		return jobs.ErrOperatorRequired
	}
	return nil
}

func nextService(s Service, values map[string]string, now time.Time) (time.Time, error) {
	if s.ScheduleSetting != "" && values[s.ScheduleSetting] != "" {
		if s.Cron != "" {
			s.Cron = values[s.ScheduleSetting]
		} else {
			n, e := strconv.Atoi(values[s.ScheduleSetting])
			if e != nil {
				return time.Time{}, ErrInvalid
			}
			s.IntervalSeconds = n
		}
	}
	if s.Cron != "" {
		if len(s.Cron) > 120 {
			return time.Time{}, ErrInvalid
		}
		schedule, e := cron.ParseStandard(s.Cron)
		if e != nil {
			return time.Time{}, ErrInvalid
		}
		next := schedule.Next(now)
		if next.IsZero() || schedule.Next(next).Sub(next) < time.Minute {
			return time.Time{}, errors.New("插件Cron不得快于每分钟或无法产生有效时间")
		}
		return next, nil
	}
	if s.IntervalSeconds < 60 || s.IntervalSeconds > 604800 {
		return time.Time{}, ErrInvalid
	}
	return now.Add(time.Duration(s.IntervalSeconds) * time.Second), nil
}
