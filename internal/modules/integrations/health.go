package integrations

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/workgate"
)

func heartbeatKind(kind string) bool {
	return kind == "open115" || kind == "clouddrive2" || kind == "qbittorrent"
}

const reauthorizeMessage = "115授权不可用，请重新扫码授权；现有授权已保留"

// Only fixed, credential-free reasons may leave the health subsystem.
func healthMessage(err error) string {
	var api *open115.APIError
	switch {
	case err == nil:
		return "连接成功"
	case errors.Is(err, open115.ErrAuthorization):
		return reauthorizeMessage
	case errors.Is(err, open115.ErrRateLimited):
		return "115接口限流或刷新冷却中，请稍后检测"
	case errors.As(err, &api):
		if api.TerminalRefresh() || api.ExpiredAccess() {
			return reauthorizeMessage
		}
		if api.RateLimited() {
			return "115接口限流，请稍后检测"
		}
	case errors.Is(err, context.DeadlineExceeded):
		return "连接检测超时，请检查网络后重试"
	}
	return "连接检测失败，请检查网络、凭据和服务状态"
}
func safeHealthMessage(message string) string {
	for _, s := range []string{"连接成功", reauthorizeMessage, "115接口限流或刷新冷却中，请稍后检测", "115接口限流，请稍后检测", "连接检测超时，请检查网络后重试", "连接检测失败，请检查网络、凭据和服务状态"} {
		if message == s {
			return s
		}
	}
	return "连接检测失败，请检查网络、凭据和服务状态"
}
func healthDelay(kind string, failures int, err error) time.Duration {
	base, jitter := time.Minute, 15*time.Second
	if kind == "open115" {
		base, jitter = time.Hour, 10*time.Minute
	}
	if failures > 0 {
		base = min(30*time.Minute, time.Minute*time.Duration(1<<min(failures-1, 5)))
		if kind == "open115" {
			base = min(6*time.Hour, 30*time.Minute*time.Duration(1<<min(failures-1, 4)))
		}
	}
	var api *open115.APIError
	if errors.Is(err, open115.ErrRateLimited) || errors.As(err, &api) && api.RateLimited() {
		base = max(base, 6*time.Hour)
		if api != nil {
			base = max(base, time.Until(api.RetryAt))
		}
	}
	return base + time.Duration(rand.Int64N(int64(jitter)))
}

// ScheduleHealth preserves persistent cooldowns; only a new config revision is due immediately.
// The upsert must not shorten an unchanged revision: EXCLUDED may reflect a pre-completion snapshot
// after waiting for the probe transaction to release the schedule row.
func (m *Manager) ScheduleHealth(ctx context.Context) error {
	_, err := m.Repository.DB.Exec(ctx, `INSERT INTO integration_health_schedule(integration_id,config_revision,next_check_at)
 SELECT id,revision,CASE WHEN last_check_state='ready' AND last_checked_at>=updated_at THEN last_checked_at+CASE WHEN kind='open115' THEN interval '70 minutes' ELSE interval '75 seconds' END ELSE clock_timestamp() END
 FROM integrations WHERE enabled AND kind IN('open115','clouddrive2','qbittorrent')
 ON CONFLICT(integration_id) DO UPDATE SET config_revision=EXCLUDED.config_revision,
 next_check_at=clock_timestamp(),failures=0
 WHERE coalesce(integration_health_schedule.lease_until,'epoch')<clock_timestamp()
 AND integration_health_schedule.config_revision<>EXCLUDED.config_revision`)
	if err != nil {
		return err
	}
	// Normalize only obsolete success cadences beyond the current maximum. A
	// freshly completed probe is always inside this bound, even if i was read
	// before completion; failure backoff and active leases are never shortened.
	_, err = m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule h SET
 next_check_at=CASE WHEN i.last_check_state='ready' AND i.last_checked_at>=i.updated_at THEN i.last_checked_at+CASE WHEN i.kind='open115' THEN interval '70 minutes' ELSE interval '75 seconds' END ELSE clock_timestamp() END
 FROM integrations i WHERE i.id=h.integration_id AND i.enabled
 AND i.kind IN('open115','clouddrive2','qbittorrent') AND h.config_revision=i.revision
 AND h.failures=0 AND coalesce(h.lease_until,'epoch')<clock_timestamp()
 AND h.next_check_at>clock_timestamp()+CASE WHEN i.kind='open115' THEN interval '70 minutes' ELSE interval '75 seconds' END`)
	return err
}

// RunHealth is joined on shutdown; PG fencing, not process-local timers, owns the schedule.
func (m *Manager) RunHealth(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var lastWarning time.Time
	for {
		release, err := workgate.BeginWork(ctx, m.Repository.DB)
		if err == nil {
			err = m.ScheduleHealth(ctx)
			if err == nil {
				results := make(chan error, 3)
				for range 3 {
					go func() { _, e := m.checkHealth(ctx, "", false, logger); results <- e }()
				}
				for range 3 {
					if e := <-results; e != nil {
						err = e
					}
				}
			}
			release()
		}
		if err != nil && ctx.Err() == nil && time.Since(lastWarning) >= time.Hour {
			logger.Warn("connection heartbeat scheduling unavailable", "module", "integrations")
			lastWarning = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.healthWake:
		}
	}
}

func (m *Manager) checkHealth(ctx context.Context, id string, manual bool, logger *slog.Logger) (CheckResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token := jobs.UUID()
	var selected string
	err := m.Repository.DB.QueryRow(ctx, `WITH candidate AS(
 SELECT h.integration_id FROM integration_health_schedule h JOIN integrations i ON i.id=h.integration_id
 WHERE i.enabled AND i.kind IN('open115','clouddrive2','qbittorrent') AND ($1='' OR i.id::text=$1)
 AND coalesce(h.lease_until,'epoch')<clock_timestamp()
 AND (h.next_check_at<=clock_timestamp() OR ($2 AND (i.last_checked_at IS NULL OR i.last_checked_at<clock_timestamp()-CASE WHEN i.kind='open115' THEN interval '10 minutes' ELSE interval '10 seconds' END)
 AND i.last_check_message NOT IN('115接口限流，请稍后检测','115接口限流或刷新冷却中，请稍后检测')))
 ORDER BY h.next_check_at FOR UPDATE OF h SKIP LOCKED LIMIT 1)
 UPDATE integration_health_schedule h SET lease_token=$3,lease_until=clock_timestamp()+interval '30 seconds'
 FROM candidate c WHERE h.integration_id=c.integration_id RETURNING h.integration_id::text`, id, manual, token).Scan(&selected)
	if errors.Is(err, pgx.ErrNoRows) && manual {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			var active bool
			if e := m.Repository.DB.QueryRow(ctx, `SELECT coalesce(lease_until>clock_timestamp(),false) FROM integration_health_schedule WHERE integration_id=$1`, id).Scan(&active); e != nil {
				return CheckResult{}, ErrUnavailable
			}
			if !active {
				c, e := m.Repository.Get(ctx, id)
				return CheckResult{Passed: c.Record.LastCheckState == "ready", State: c.Record.Readiness.State, Message: c.Record.LastCheckMessage, Reused: true}, e
			}
			select {
			case <-ctx.Done():
				return CheckResult{}, ErrUnavailable
			case <-ticker.C:
			}
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckResult{}, nil
	}
	if err != nil {
		return CheckResult{}, err
	}
	c, err := m.Repository.Get(ctx, selected)
	if err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{State: "unavailable"}
	var probeErr error
	if c.Record.Enabled && c.Record.Kind == "open115" {
		after, _ := time.Parse(time.RFC3339Nano, c.Values["refreshAfter"])
		switch {
		case c.Values["refreshBlocked"] != "":
			probeErr = open115.ErrAuthorization
		case time.Now().Before(after):
			probeErr = m.officialClient().Probe(ctx, c.Values["accessToken"])
		default:
			probeErr = m.officialCall(ctx, selected, true, func(access string) error { return m.officialClient().Probe(ctx, access) })
		}

	} else if c.Record.Enabled {
		result, probeErr = m.ProbeCredentials(ctx, c)
		if probeErr == nil && !result.Passed {
			probeErr = errors.New("probe failed")
		}
	}
	if ctx.Err() != nil {
		probeErr = ctx.Err()
	}
	result = CheckResult{Passed: probeErr == nil && c.Record.Enabled, State: "unavailable", Message: healthMessage(probeErr)}
	if result.Passed {
		result.State = "ready"
	}
	done, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	tx, err := m.Repository.DB.Begin(done)
	if err != nil {
		return CheckResult{}, err
	}
	defer deferutil.Rollback(done, tx)
	// Match config-save lock order (integration, then schedule), avoiding a deadlock.
	var revision int64
	if err = tx.QueryRow(done, `SELECT revision FROM integrations WHERE id=$1 FOR UPDATE`, selected).Scan(&revision); err != nil {
		return CheckResult{}, err
	}
	var failures int
	if err = tx.QueryRow(done, `SELECT failures FROM integration_health_schedule WHERE integration_id=$1 AND lease_token=$2 AND lease_until>clock_timestamp() FOR UPDATE`, selected, token).Scan(&failures); err != nil {
		return CheckResult{}, err
	}
	if result.Passed {
		failures = 0
	} else {
		failures++
	}
	tag, err := tx.Exec(done, `UPDATE integrations SET last_check_state=$3,last_check_message=$4,last_checked_at=clock_timestamp() WHERE id=$1 AND revision=$2 AND enabled`, selected, c.Record.Revision, result.State, result.Message)
	if err != nil {
		return CheckResult{}, err
	}
	if _, err = tx.Exec(done, `UPDATE integration_health_schedule SET failures=$3,config_revision=$4,next_check_at=CASE WHEN EXISTS(SELECT 1 FROM integrations WHERE id=$1 AND revision=$4) THEN clock_timestamp()+($5::bigint*interval '1 millisecond') ELSE clock_timestamp() END,lease_token=NULL,lease_until=NULL WHERE integration_id=$1 AND lease_token=$2`, selected, token, failures, c.Record.Revision, healthDelay(c.Record.Kind, failures, probeErr).Milliseconds()); err != nil {
		return CheckResult{}, err
	}
	if err = tx.Commit(done); err != nil {
		return CheckResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return CheckResult{}, ErrRevision
	}
	if logger != nil && (c.Record.LastCheckedAt == nil || c.Record.LastCheckState != result.State) {
		logger.Info("connection heartbeat state changed", "module", "integrations", "kind", c.Record.Kind, "connectionId", selected, "state", result.State, "reason", result.Message)
	}
	return result, nil
}
