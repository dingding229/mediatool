package integrations

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Reproduce a scheduler INSERT whose source snapshot precedes probe completion,
// but whose ON CONFLICT waits until after completion. It must not rearm the probe.
func TestHealthScheduleStaleSnapshotCannotShortenCompletedCooldown(t *testing.T) {
	ctx, m, r := healthFixture(t)
	tx, err := m.Repository.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT integration_id FROM integration_health_schedule WHERE integration_id=$1 FOR UPDATE`, r.ID); err != nil {
		t.Fatal(err)
	}
	scheduled := make(chan error, 1)
	go func() { scheduled <- m.ScheduleHealth(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		err = m.Repository.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'INSERT INTO integration_health_schedule(integration_id,config_revision,next_check_at)%')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduler never reached conflict lock")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = tx.Exec(ctx, `UPDATE integrations SET last_check_state='ready',last_check_message='连接成功',last_checked_at=clock_timestamp() WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	var next time.Time
	if err = tx.QueryRow(ctx, `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()+interval '65 minutes' WHERE integration_id=$1 RETURNING next_check_at`, r.ID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-scheduled; err != nil {
		t.Fatal(err)
	}
	var actual time.Time
	if err = m.Repository.DB.QueryRow(ctx, `SELECT next_check_at FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if !actual.Equal(next) {
		t.Fatalf("completion cooldown changed: %v -> %v", next, actual)
	}
	var calls atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	out, err := m.Test(ctx, r.ID)
	if err != nil || !out.Passed || !out.Reused || calls.Load() != 0 {
		t.Fatal(out, err, calls.Load())
	}
}
