package maintenance

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"media-tool/internal/platform/scheduling"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const SampleCron = "* * * * *"

func (e Engine) Schedule(ctx context.Context) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(91220881)`).Scan(&locked); err != nil || !locked {
		return err
	}
	r := PolicyRecord{Value: Defaults()}
	var policyRaw []byte
	if err = tx.QueryRow(ctx, `SELECT value,revision,next_backup_at FROM ops_policy WHERE singleton FOR UPDATE`).Scan(&policyRaw, &r.Revision, &r.NextBackup); err != nil {
		return err
	}
	if err = json.Unmarshal(policyRaw, &r.Value); err != nil {
		return err
	}
	var due bool
	nextSample, _ := scheduling.Next(SampleCron, time.Now(), time.Minute)
	if err = tx.QueryRow(ctx, `UPDATE ops_policy SET next_sample_at=$1 WHERE singleton AND next_sample_at<=now() RETURNING true`, nextSample).Scan(&due); err != nil {
		due = false
	}
	backupDue := r.Value.BackupEnabled && !r.NextBackup.After(time.Now())
	if backupDue {
		key := "cron:" + strconv.FormatInt(r.Revision, 10) + ":" + r.NextBackup.UTC().Format(time.RFC3339Nano)
		if _, queueErr := e.QueueBackup(ctx, key); queueErr != nil {
			if queueErr == ErrConflict {
				backupDue = false
			} else {
				return queueErr
			}
		}
		if backupDue {
			next, nextErr := scheduling.Next(r.Value.BackupCron, time.Now(), time.Hour)
			if nextErr != nil {
				return nextErr
			}
			if _, err = tx.Exec(ctx, `UPDATE ops_policy SET next_backup_at=$1 WHERE singleton`, next); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if due {
		if err = e.Sample(ctx, r.Value); err != nil {
			return err
		}
		if err = e.Retain(ctx, r.Value); err != nil {
			return err
		}
	}

	return err
}
func (e Engine) Sample(ctx context.Context, p Policy) error {
	var process runtime.MemStats
	runtime.ReadMemStats(&process)
	v := map[string]any{"processBytes": process.HeapAlloc, "hostScope": "只读宿主proc；磁盘为STRM文件系统", "sampledAt": time.Now()}
	proc := e.HostProc
	if proc == "" {
		proc = "/proc"
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "meminfo")); err == nil {
		mem := map[string]float64{}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) > 1 {
				n, _ := strconv.ParseFloat(f[1], 64)
				mem[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		}
		if mem["MemTotal"] > 0 && mem["MemAvailable"] > 0 {
			v["memoryPercent"] = (1 - mem["MemAvailable"]/mem["MemTotal"]) * 100
		}
	}
	cpus := 0
	if raw, err := os.ReadFile(filepath.Join(proc, "stat")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) > 0 && len(f[0]) > 3 && strings.HasPrefix(f[0], "cpu") {
				cpus++
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "loadavg")); err == nil {
		f := strings.Fields(string(raw))
		if len(f) >= 3 {
			load, parseErr := strconv.ParseFloat(f[0], 64)
			if parseErr == nil {
				v["load1"] = load
				if cpus > 0 {
					v["loadPerCPU"] = load / float64(cpus)
					v["hostCPUs"] = cpus
				}
			}
		}
	}
	var disk unix.Statfs_t
	if unix.Statfs(e.OutputRoot, &disk) == nil {
		v["diskFreeGB"] = float64(disk.Bavail) * float64(disk.Bsize) / (1 << 30)
	}
	var offline int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM (VALUES('default'),('transfer'),('library'),('maintenance'))q(name) WHERE NOT EXISTS(SELECT 1 FROM worker_heartbeats w WHERE w.queue=q.name AND last_seen_at>now()-interval '20 seconds')`).Scan(&offline); err != nil {
		return err
	}
	v["offlineQueues"] = offline
	var failed, succeeded, pending int
	var avg *float64
	if err := e.DB.QueryRow(ctx, `SELECT count(*)FILTER(WHERE state='failed' AND updated_at>now()-interval '10 minutes'),count(*)FILTER(WHERE state='succeeded' AND updated_at>now()-interval '10 minutes'),count(*)FILTER(WHERE state IN('pending','retry','running')),avg(extract(epoch FROM(updated_at-started_at)))FILTER(WHERE state IN('succeeded','failed')AND started_at IS NOT NULL AND updated_at>now()-interval '10 minutes') FROM jobs`).Scan(&failed, &succeeded, &pending, &avg); err != nil {
		return err
	}
	v["failedJobs10m"] = failed
	v["succeededJobs10m"] = succeeded
	v["pendingJobs"] = pending
	v["jobSeconds"] = avg
	raw, _ := json.Marshal(v)
	if _, err := e.DB.Exec(ctx, `INSERT INTO ops_samples(value)VALUES($1)`, raw); err != nil {
		return err
	}
	if _, err := e.DB.Exec(ctx, `DELETE FROM ops_samples WHERE sampled_at<now()-make_interval(days=>$1)`, p.SampleDays); err != nil {
		return err
	}
	return e.evaluate(ctx, p, v)
}
func number(v any) *float64 {
	var n float64
	switch x := v.(type) {
	case int:
		n = float64(x)
	case float64:
		n = x
	default:
		return nil
	}
	return &n
}
func (e Engine) evaluate(ctx context.Context, p Policy, v map[string]any) error {
	rules := []struct {
		code, key, message string
		threshold          float64
		below              bool
	}{{"memory", "memoryPercent", "宿主内存使用率过高", p.MemoryPercent, false}, {"disk", "diskFreeGB", "STRM所在文件系统剩余空间不足", p.DiskFreeGB, true}, {"load", "loadPerCPU", "宿主每CPU负载过高", p.LoadPerCPU, false}, {"workers", "offlineQueues", "业务Worker离线", 1, false}, {"failures", "failedJobs10m", "十分钟内失败任务过多", float64(p.FailedJobs), false}}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, rule := range rules {
		if _, err = tx.Exec(ctx, `INSERT INTO ops_alerts(code)VALUES($1)ON CONFLICT DO NOTHING`, rule.code); err != nil {
			return err
		}
		var state string
		var hits int
		var sent, ack *time.Time
		if err = tx.QueryRow(ctx, `SELECT state,hits,last_sent_at,acknowledged_at FROM ops_alerts WHERE code=$1 FOR UPDATE`, rule.code).Scan(&state, &hits, &sent, &ack); err != nil {
			return err
		}
		value := number(v[rule.key])
		message := rule.message
		emit := ""
		first := false
		if !p.AlertsEnabled {
			state = "disabled"
			hits = 0
			ack = nil
			message = "告警策略已关闭"
		} else if value == nil {
			message = "指标不可用；不把缺失值当作正常"
			if state != "active" {
				state = "unknown"
				hits = 0
			}
		} else {
			bad := *value >= rule.threshold
			if rule.below {
				bad = *value <= rule.threshold
			}
			if bad && p.AlertsEnabled {
				hits++
				if state != "active" && hits >= p.Consecutive {
					state = "active"
					first = true
					ack = nil
					emit = "ops.alert.active"
				} else if state == "active" && ack == nil && (sent == nil || time.Since(*sent) >= time.Duration(p.CooldownMinutes)*time.Minute) {
					emit = "ops.alert.active"
				} else if state != "active" {
					state = "pending"
				}
			} else {
				if state == "active" && p.AlertsEnabled {
					emit = "ops.alert.recovered"
				}
				state = "normal"
				hits = 0
				ack = nil
				message = "指标已恢复或告警已关闭"
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE ops_alerts SET state=$2,hits=$3,value=$4,last_seen_at=now(),message=$5,first_seen_at=CASE WHEN $6 THEN now() ELSE first_seen_at END,acknowledged_at=$7,last_sent_at=CASE WHEN $8<>'' THEN now()ELSE last_sent_at END WHERE code=$1`, rule.code, state, hits, value, message, first, ack, emit); err != nil {
			return err
		}
		if emit != "" {
			raw, _ := json.Marshal(map[string]string{"title": rule.message, "message": message + "；指标 " + rule.key + "，阈值 " + strconv.FormatFloat(rule.threshold, 'f', 2, 64)})
			if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,$2,$1,$3)`, newID(), emit, raw); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
