package integrations

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"math/rand/v2"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"reflect"
	"time"
)

// TickOfficialHealth is called by the existing non-overlapping scheduler. PG
// deadlines survive restarts; a fenced lease coordinates other app processes.
// It only uses the existing account-info Probe, never listing or download links.
func (m *Manager) TickOfficialHealth(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := m.Repository.DB.Exec(ctx, `INSERT INTO integration_health_schedule(integration_id,config_revision,next_check_at) SELECT id,revision,clock_timestamp()+($1::bigint*interval '1 millisecond') FROM integrations WHERE kind='open115' AND enabled ON CONFLICT(integration_id) DO UPDATE SET config_revision=EXCLUDED.config_revision,next_check_at=EXCLUDED.next_check_at,failures=0 WHERE integration_health_schedule.config_revision<>EXCLUDED.config_revision AND coalesce(integration_health_schedule.lease_until,'epoch')<clock_timestamp()`, (5*time.Minute + time.Duration(rand.Int64N(int64(5*time.Minute)))).Milliseconds())
	if err != nil {
		return err
	}
	if time.Now().Before(m.healthNotBefore) {
		return nil
	}
	token := jobs.UUID()
	var id string
	err = m.Repository.DB.QueryRow(ctx, `WITH candidate AS(SELECT h.integration_id FROM integration_health_schedule h JOIN integrations i ON i.id=h.integration_id WHERE i.kind='open115' AND i.enabled AND h.next_check_at<=clock_timestamp() AND coalesce(h.lease_until,'epoch')<clock_timestamp() ORDER BY h.next_check_at FOR UPDATE OF h SKIP LOCKED LIMIT 1) UPDATE integration_health_schedule h SET lease_token=$1,lease_until=clock_timestamp()+interval '2 minutes',next_check_at=clock_timestamp()+interval '10 minutes' FROM candidate c WHERE h.integration_id=c.integration_id RETURNING h.integration_id::text`, token).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := m.Repository.Get(ctx, id)
	result := CheckResult{State: "unavailable", Message: "115定时检测未完成，保留现有授权"}
	if err == nil {
		// Authorization refresh has its own persistent lock; missing grants must
		// not cause a request with empty credentials or a new authorization flow.
		if c.Values["accessToken"] == "" {
			result.Message = "115授权缺失，请明确扫码授权"
		} else {
			var token string
			token, err = m.officialToken(ctx, c.Record.ID, "", true)
			if err == nil {
				var fresh Credentials
				fresh, err = m.Repository.Get(ctx, c.Record.ID)
				if err == nil && fresh.Values["accessToken"] != token {
					err = ErrRevision
				}
				if err == nil {
					c = fresh
					err = m.officialClient().Probe(ctx, token)
					if err == nil {
						result = CheckResult{Passed: true, State: "ready", Message: "115官方授权轻量健康检测通过；不代表播放器验收"}
					}
				}
			}
		}
	}
	if err != nil {
		// A terminal/uncertain token refresh may itself increment the revision
		// while preserving the token pair. Publish that failure only if the
		// same configuration/authorization is still selected, never a new one.
		if fresh, e := m.Repository.Get(ctx, id); e == nil && fresh.Values["accessToken"] == c.Values["accessToken"] && fresh.Values["refreshToken"] == c.Values["refreshToken"] && fresh.Record.Enabled == c.Record.Enabled && fresh.Record.Name == c.Record.Name && reflect.DeepEqual(fresh.Record.Settings, c.Record.Settings) {
			c = fresh
		}
		result = CheckResult{State: "unavailable", Message: "115定时检测失败：" + err.Error()}
	}
	done, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	tx, err := m.Repository.DB.Begin(done)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(done, tx)
	var failures int
	err = tx.QueryRow(done, `SELECT failures FROM integration_health_schedule WHERE integration_id=$1 AND lease_token=$2 AND lease_until>clock_timestamp() FOR UPDATE`, id, token).Scan(&failures)
	if err != nil {
		return err
	}
	delay := 6*time.Hour + time.Duration(rand.Int64N(int64(time.Hour)))
	if !result.Passed {
		failures++
		delay = min(6*time.Hour, 30*time.Minute*time.Duration(1<<min(failures-1, 4))) + time.Duration(rand.Int64N(int64(10*time.Minute)))
	} else {
		failures = 0
	}
	if c.Record.ID != "" {
		if _, err = tx.Exec(done, `UPDATE integrations SET last_check_state=$3,last_check_message=$4,last_checked_at=clock_timestamp() WHERE id=$1 AND revision=$2`, id, c.Record.Revision, result.State, result.Message); err != nil {
			return err
		}
	}
	// If config changed during the probe, the stale receipt is never published;
	// test the new config after startup-style delay, not six hours later.
	if _, err = tx.Exec(done, `UPDATE integration_health_schedule SET failures=$3,config_revision=$5,next_check_at=CASE WHEN EXISTS(SELECT 1 FROM integrations WHERE id=$1 AND revision=$5) THEN clock_timestamp()+($4::bigint*interval '1 millisecond') ELSE clock_timestamp()+interval '5 minutes' END,lease_token=NULL,lease_until=NULL WHERE integration_id=$1 AND lease_token=$2`, id, token, failures, delay.Milliseconds(), c.Record.Revision); err != nil {
		return err
	}
	return tx.Commit(done)
}
