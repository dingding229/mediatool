package integrations

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOfficialHealthPersistentDueLeaseBackoffAndRevision(t *testing.T) {
	ctx, m, record := officialFixture(t)
	m.healthNotBefore = time.Time{}
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE integrations SET enabled=true WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/open/user/info" {
			t.Fatalf("heavy probe called: %s", r.URL.Path)
		}
		calls.Add(1)
		return officialReply(r, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	if err := m.TickOfficialHealth(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("startup probe not delayed")
	}
	var delay float64
	if err := m.Repository.DB.QueryRow(ctx, `SELECT extract(epoch FROM(next_check_at-clock_timestamp())) FROM integration_health_schedule WHERE integration_id=$1`, record.ID).Scan(&delay); err != nil || delay < 290 || delay > 610 {
		t.Fatal("first check jitter", delay, err)
	}
	due := func() {
		t.Helper()
		if _, err := m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()-interval '1 second',lease_until=NULL WHERE integration_id=$1`, record.ID); err != nil {
			t.Fatal(err)
		}
	}
	due()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	// Two managers share PG, never two independent test suites/databases.
	other := NewManager(m.Repository, m.Config)
	other.healthNotBefore = time.Time{}
	other.OutboundHTTP = m.OutboundHTTP
	defer other.Close()
	for _, manager := range []*Manager{m, other} {
		wg.Go(func() { errs <- manager.TickOfficialHealth(ctx) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate health probe", calls.Load())
	}
	saved, err := m.Repository.Get(ctx, record.ID)
	if err != nil || saved.Record.LastCheckState != "ready" || saved.Record.LastCheckedAt == nil {
		t.Fatal("receipt absent", err)
	}
	if err = other.TickOfficialHealth(ctx); err != nil || calls.Load() != 1 {
		t.Fatal("restart ignored schedule", err)
	}
	due()
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("owned health fixture offline")
	})}
	if err = m.TickOfficialHealth(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err = m.Repository.Get(ctx, record.ID)
	if err != nil || saved.Record.LastCheckState == "ready" || saved.Record.LastCheckedAt == nil || saved.Values["accessToken"] != "old-access" || saved.Values["refreshToken"] != "old-refresh" {
		t.Fatal("failure erased authorization or hid receipt", err)
	}
	if err = m.Repository.DB.QueryRow(ctx, `SELECT extract(epoch FROM(next_check_at-clock_timestamp())) FROM integration_health_schedule WHERE integration_id=$1`, record.ID).Scan(&delay); err != nil || delay < 1700 || delay > 2500 {
		t.Fatal("backoff", delay, err)
	}
	// Config change schedules a delayed probe and invalidates the prior result.
	if _, err = m.Repository.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	n := calls.Load()
	if err = m.TickOfficialHealth(ctx); err != nil || calls.Load() != n {
		t.Fatal("configuration change caused immediate burst", err)
	}
}

func TestOfficialHealthStaleProbeCannotValidateNewConfiguration(t *testing.T) {
	ctx, m, record := officialFixture(t)
	m.healthNotBefore = time.Time{}
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE integrations SET enabled=true WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.TickOfficialHealth(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()-interval '1 second' WHERE integration_id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return officialReply(r, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	done := make(chan error, 1)
	go func() { done <- m.TickOfficialHealth(ctx) }()
	<-entered
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1,last_check_state='unchecked',last_checked_at=NULL,updated_at=clock_timestamp() WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	saved, err := m.Repository.Get(ctx, record.ID)
	if err != nil || saved.Record.LastCheckedAt != nil || saved.Record.LastCheckState != "unchecked" {
		t.Fatal("stale authorization validated new config", err)
	}
}

func TestOfficialHealthFailedRefreshKeepsTokensAndPublishesFailure(t *testing.T) {
	ctx, m, record := officialFixture(t)
	m.healthNotBefore = time.Time{}
	current, err := m.Repository.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Values["accessExpiresAt"] = "2000-01-01T00:00:00Z"
	encrypted, err := m.Repository.Vault.Seal(record.ID, current.Values)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Repository.DB.Exec(ctx, `UPDATE integrations SET enabled=true,credentials_ciphertext=$2 WHERE id=$1`, record.ID, encrypted); err != nil {
		t.Fatal(err)
	}
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		return officialReply(r, `{"state":0,"code":40140120,"message":"owned refresh fixture rejected"}`), nil
	})}
	if err = m.TickOfficialHealth(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()-interval '1 second' WHERE integration_id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.TickOfficialHealth(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err := m.Repository.Get(ctx, record.ID)
	if err != nil || saved.Record.LastCheckedAt == nil || saved.Record.LastCheckState != "unavailable" || saved.Values["accessToken"] != "old-access" || saved.Values["refreshToken"] != "old-refresh" || saved.Values["refreshBlocked"] != "40140120" {
		t.Fatal("refresh failure lost receipt or authorization", saved.Record, err)
	}
}

func TestOfficialHealthRestartDefersEvenAnOverduePersistedCheck(t *testing.T) {
	ctx, m, record := officialFixture(t)
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE integrations SET enabled=true WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Repository.DB.Exec(ctx, `INSERT INTO integration_health_schedule(integration_id,config_revision,next_check_at)VALUES($1,$2,clock_timestamp()-interval '1 hour')`, record.ID, record.Revision); err != nil {
		t.Fatal(err)
	}
	var calls int
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return officialReply(r, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	if err := m.TickOfficialHealth(ctx); err != nil || calls != 0 {
		t.Fatal("restart immediately probed overdue schedule", calls, err)
	}
	m.healthNotBefore = time.Time{}
	if err := m.TickOfficialHealth(ctx); err != nil || calls != 1 {
		t.Fatal("overdue check not resumed after startup barrier", calls, err)
	}
}
