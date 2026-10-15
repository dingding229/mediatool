package system

import (
	"context"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Probe struct {
	State     string `json:"state"`
	LatencyMS int64  `json:"latencyMs"`
	Message   string `json:"message,omitempty"`
}
type Worker struct {
	Owner    string    `json:"owner"`
	Queue    string    `json:"queue"`
	LastSeen time.Time `json:"lastSeenAt"`
	Online   bool      `json:"online"`
}
type Queue struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Count   int64  `json:"count"`
	Expired int64  `json:"expiredLeases"`
}
type Schedule struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Cron     string     `json:"cron"`
	Enabled  bool       `json:"enabled"`
	Next     *time.Time `json:"nextRunAt"`
	Last     *time.Time `json:"lastRunAt,omitempty"`
	Error    string     `json:"error,omitempty"`
	Link     string     `json:"link"`
	Timezone string     `json:"timezone"`
}
type Snapshot struct {
	SampledAt            time.Time         `json:"sampledAt"`
	PostgreSQL           Probe             `json:"postgres"`
	Redis                Probe             `json:"redis"`
	Migration            *int64            `json:"migration"`
	DatabaseBytes        *int64            `json:"databaseBytes"`
	PendingNotifications *int64            `json:"pendingNotifications"`
	Pool                 map[string]int32  `json:"pool"`
	Process              map[string]uint64 `json:"process"`
	Workers              []Worker          `json:"workers"`
	Queues               []Queue           `json:"queues"`
	Schedules            []Schedule        `json:"schedules"`
	Unavailable          []string          `json:"unavailable"`
}

// Read-only and bounded: no pruning, task replay, credential probing, or external provider calls.
func Inspect(ctx context.Context, db *pgxpool.Pool, cache *redis.Client) Snapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out := Snapshot{SampledAt: time.Now(), PostgreSQL: Probe{State: "unavailable"}, Redis: Probe{State: "unavailable"}, Workers: []Worker{}, Queues: []Queue{}, Schedules: []Schedule{}, Unavailable: []string{}, Process: map[string]uint64{"heapBytes": mem.HeapAlloc, "allocatedBytes": mem.Sys, "goroutines": uint64(runtime.NumGoroutine())}}
	if cache != nil {
		check, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		start := time.Now()
		err := cache.Ping(check).Err()
		cancel()
		out.Redis.LatencyMS = time.Since(start).Milliseconds()
		if err == nil {
			out.Redis.State = "ready"
		} else {
			out.Redis.Message = err.Error()
		}
	}
	if db == nil {
		out.Unavailable = append(out.Unavailable, "database")
		return out
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	err := db.Ping(check)
	out.PostgreSQL.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		out.PostgreSQL.Message = err.Error()
		out.Unavailable = append(out.Unavailable, "database")
		return out
	}
	out.PostgreSQL.State = "ready"
	stats := db.Stat()
	out.Pool = map[string]int32{"total": stats.TotalConns(), "acquired": stats.AcquiredConns(), "idle": stats.IdleConns(), "max": stats.MaxConns()}
	for _, q := range []struct {
		name, sql string
		target    **int64
	}{
		{"migration", `SELECT max(version_id) FROM goose_db_version WHERE is_applied`, &out.Migration},
		{"databaseBytes", `SELECT pg_database_size(current_database())`, &out.DatabaseBytes},
		{"pendingNotifications", `SELECT count(*) FROM notification_deliveries WHERE state IN ('pending','retry','sending','failed')`, &out.PendingNotifications},
	} {
		if err := db.QueryRow(check, q.sql).Scan(q.target); err != nil {
			out.Unavailable = append(out.Unavailable, q.name)
		}
	}
	rows, err := db.Query(check, `SELECT owner,queue,last_seen_at,last_seen_at>now()-interval '20 seconds' FROM worker_heartbeats ORDER BY last_seen_at DESC LIMIT 100`)
	if err != nil {
		out.Unavailable = append(out.Unavailable, "workers")
	} else {
		for rows.Next() {
			var w Worker
			if rows.Scan(&w.Owner, &w.Queue, &w.LastSeen, &w.Online) != nil {
				out.Unavailable = append(out.Unavailable, "workers")
				break
			}
			out.Workers = append(out.Workers, w)
		}
		if rows.Err() != nil {
			out.Unavailable = append(out.Unavailable, "workers")
		}
		rows.Close()
	}
	rows, err = db.Query(check, `SELECT queue,state,count(*),count(*)FILTER(WHERE state='running' AND lease_until<now()) FROM jobs GROUP BY queue,state ORDER BY queue,state`)
	if err != nil {
		out.Unavailable = append(out.Unavailable, "queues")
	} else {
		for rows.Next() {
			var q Queue
			if rows.Scan(&q.Name, &q.State, &q.Count, &q.Expired) != nil {
				out.Unavailable = append(out.Unavailable, "queues")
				break
			}
			out.Queues = append(out.Queues, q)
		}
		if rows.Err() != nil {
			out.Unavailable = append(out.Unavailable, "queues")
		}
		rows.Close()
	}
	// UNION uses actual persisted due times, never recalculates dispatch history.
	rows, err = db.Query(check, `SELECT 'sites','PT站点刷新',cron_expression,enabled,next_run_at,last_dispatched_at,'','/sites' FROM site_metric_schedule
 UNION ALL SELECT id::text,'云盘扫描 · '||root,cron_expression,enabled,next_run_at,NULL::timestamptz,last_error,'/cloudStorage' FROM library_scan_schedules
 UNION ALL SELECT s.id::text,'订阅 · '||s.title,coalesce(a.value->>'subscriptionCron','*/30 * * * *'),s.enabled,s.next_run_at,NULL::timestamptz,CASE WHEN s.last_state IN ('failed','partial','error') THEN s.last_message ELSE '' END,'/subscriptions' FROM subscriptions s CROSS JOIN application_settings a WHERE s.archived_at IS NULL ORDER BY 1 LIMIT 300`)
	if err != nil {
		out.Unavailable = append(out.Unavailable, "schedules")
	} else {
		for rows.Next() {
			var s Schedule
			if rows.Scan(&s.ID, &s.Name, &s.Cron, &s.Enabled, &s.Next, &s.Last, &s.Error, &s.Link) != nil {
				out.Unavailable = append(out.Unavailable, "schedules")
				break
			}
			s.Timezone = "Asia/Shanghai"
			out.Schedules = append(out.Schedules, s)
		}
		if rows.Err() != nil {
			out.Unavailable = append(out.Unavailable, "schedules")
		}
		rows.Close()
	}
	return out
}
