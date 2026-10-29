package integrations

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func healthFixture(t *testing.T) (context.Context, *Manager, Record) {
	ctx, m, r := officialFixture(t)
	if _, e := m.Repository.DB.Exec(ctx, `UPDATE integrations SET enabled=true WHERE id=$1`, r.ID); e != nil {
		t.Fatal(e)
	}
	if e := m.ScheduleHealth(ctx); e != nil {
		t.Fatal(e)
	}
	return ctx, m, r
}
func dueHealth(t *testing.T, m *Manager, id string) {
	t.Helper()
	if _, e := m.Repository.DB.Exec(context.Background(), `UPDATE integration_health_schedule SET next_check_at=clock_timestamp()-interval '1 second',lease_until=NULL WHERE integration_id=$1`, id); e != nil {
		t.Fatal(e)
	}
}
func setHealthValues(t *testing.T, m *Manager, id string, extra map[string]string) map[string]string {
	t.Helper()
	c, e := m.Repository.Get(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range extra {
		c.Values[k] = v
	}
	cipher, e := m.Repository.Vault.Seal(id, c.Values)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Repository.DB.Exec(context.Background(), `UPDATE integrations SET credentials_ciphertext=$2 WHERE id=$1`, id, cipher); e != nil {
		t.Fatal(e)
	}
	return c.Values
}
func healthTick(t *testing.T, ctx context.Context, m *Manager, id string, logger *slog.Logger) CheckResult {
	t.Helper()
	r, e := m.checkHealth(ctx, id, false, logger)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestHealthTransitionStartupRestartBackoffLogsAndSanitization(t *testing.T) {
	ctx, m, r := healthFixture(t)
	var calls atomic.Int32
	var fail atomic.Bool
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Path != "/open/user/info" {
			t.Errorf("heavy probe: %s", req.URL.Path)
		}
		if fail.Load() {
			return nil, errors.New("PRIVATE cookie=secret https://user:pass@private/")
		}
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	if got := healthTick(t, ctx, m, r.ID, logger); !got.Passed {
		t.Fatal(got)
	}
	var delay float64
	m.Repository.DB.QueryRow(ctx, `SELECT extract(epoch FROM(next_check_at-clock_timestamp())) FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&delay)
	if delay < 3590 || delay > 4210 {
		t.Fatal("one-hour jitter", delay)
	}
	// A new manager preserves due dates; restart must not bypass cooldown.
	other := NewManager(m.Repository, m.Config)
	defer other.Close()
	other.OutboundHTTP = m.OutboundHTTP
	if e := other.ScheduleHealth(ctx); e != nil {
		t.Fatal(e)
	}
	healthTick(t, ctx, other, r.ID, logger)
	if calls.Load() != 1 {
		t.Fatal("restart duplicate", calls.Load())
	}
	fail.Store(true)
	dueHealth(t, m, r.ID)
	got := healthTick(t, ctx, m, r.ID, logger)
	if got.Passed || strings.Contains(got.Message, "PRIVATE") {
		t.Fatal(got)
	}
	dueHealth(t, m, r.ID)
	healthTick(t, ctx, m, r.ID, logger)
	if strings.Count(logs.String(), "state changed") != 2 || strings.Contains(logs.String(), "PRIVATE") {
		t.Fatal(logs.String())
	}
	fail.Store(false)
	dueHealth(t, m, r.ID)
	healthTick(t, ctx, m, r.ID, logger)
	if strings.Count(logs.String(), "state changed") != 3 {
		t.Fatal(logs.String())
	}
	c, _ := m.Repository.Get(ctx, r.ID)
	if c.Record.Revision != r.Revision || !c.Record.Enabled {
		t.Fatal("health edited config")
	}
}
func TestHealthExpiredRefreshAndBusinessShareSingleLock(t *testing.T) {
	ctx, m, r := healthFixture(t)
	setHealthValues(t, m, r.ID, map[string]string{"accessExpiresAt": time.Now().Add(-time.Hour).Format(time.RFC3339Nano)})
	var refresh, probe atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/open/refreshToken":
			refresh.Add(1)
			close(started)
			<-release
			return officialReply(req, `{"state":1,"data":{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":7200}}`), nil
		case "/open/user/info":
			probe.Add(1)
			if req.Header.Get("Authorization") != "Bearer fresh-access" {
				t.Error("old token")
			}
			return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
		}
		t.Errorf("unexpected request %s", req.URL.Path)
		return nil, errors.New("forbidden")
	})}
	other := NewManager(m.Repository, m.Config)
	defer other.Close()
	other.OutboundHTTP = m.OutboundHTTP
	business := make(chan error, 1)
	go func() { _, e := other.officialToken(ctx, r.ID, "", true); business <- e }()
	<-started
	heartbeat := make(chan error, 1)
	go func() {
		got, e := m.Test(ctx, r.ID)
		if e == nil && !got.Passed {
			e = errors.New("heartbeat failed")
		}
		heartbeat <- e
	}()
	close(release)
	if e := <-business; e != nil {
		t.Fatal(e)
	}
	if e := <-heartbeat; e != nil {
		t.Fatal(e)
	}
	if refresh.Load() != 1 || probe.Load() != 1 {
		t.Fatal(refresh.Load(), probe.Load())
	}
	c, _ := m.Repository.Get(ctx, r.ID)
	if c.Record.LastCheckState != "ready" || c.Record.Revision != r.Revision || !c.Record.UpdatedAt.Equal(r.UpdatedAt) {
		t.Fatal(c.Record)
	}
}
func TestHealthRefreshFailurePreservesTokensConfigurationAndBlocksReplay(t *testing.T) {
	ctx, m, r := healthFixture(t)
	before := setHealthValues(t, m, r.ID, map[string]string{"accessExpiresAt": time.Now().Add(-time.Hour).Format(time.RFC3339Nano)})
	var calls atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Path != "/open/refreshToken" {
			t.Error("unexpected probe")
		}
		return officialReply(req, `{"state":false,"code":40140120,"message":"PRIVATE"}`), nil
	})}
	got := healthTick(t, ctx, m, r.ID, nil)
	if got.Passed || got.Message != reauthorizeMessage {
		t.Fatal(got)
	}
	c, e := m.Repository.Get(ctx, r.ID)
	if e != nil {
		t.Fatal(e)
	}
	if c.Record.Revision != r.Revision || c.Record.Name != r.Name || !c.Record.Enabled || !c.Record.UpdatedAt.Equal(r.UpdatedAt) || !reflect.DeepEqual(c.Record.Settings, r.Settings) {
		t.Fatal("configuration changed")
	}
	for key, value := range before {
		if c.Values[key] != value {
			t.Fatalf("changed existing field %s", key)
		}
	}
	for key := range c.Values {
		if _, ok := before[key]; !ok && key != "refreshAfter" && key != "refreshBlocked" {
			t.Fatalf("unexpected field %s", key)
		}
	}
	if c.Values["refreshBlocked"] != "40140120" {
		t.Fatal("missing safety marker")
	}
	dueHealth(t, m, r.ID)
	healthTick(t, ctx, m, r.ID, nil)
	if calls.Load() != 1 {
		t.Fatal("blocked refresh replayed")
	}
}
func TestHealthRefreshAfterOnlyUsesCurrentAccessToken(t *testing.T) {
	ctx, m, r := healthFixture(t)
	before := setHealthValues(t, m, r.ID, map[string]string{"accessExpiresAt": time.Now().Add(-time.Hour).Format(time.RFC3339Nano), "refreshAfter": time.Now().Add(time.Hour).Format(time.RFC3339Nano)})
	var calls int
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/open/user/info" || req.Header.Get("Authorization") != "Bearer old-access" {
			t.Error("cooldown refreshed")
		}
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	if !healthTick(t, ctx, m, r.ID, nil).Passed || calls != 1 {
		t.Fatal("current token was not probed")
	}
	c, _ := m.Repository.Get(ctx, r.ID)
	if !reflect.DeepEqual(c.Values, before) {
		t.Fatal("cooldown changed authorization")
	}
	dueHealth(t, m, r.ID)
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/open/user/info" {
			t.Error("expired current token caused refresh in cooldown")
		}
		return officialReply(req, `{"state":false,"code":40140125}`), nil
	})}
	if healthTick(t, ctx, m, r.ID, nil).Passed || calls != 2 {
		t.Fatal("expired token accepted or refreshed")
	}
	c, _ = m.Repository.Get(ctx, r.ID)
	if !reflect.DeepEqual(c.Values, before) {
		t.Fatal("cooldown failure changed authorization")
	}

}
func TestHealthManualScheduledConcurrentDedupAndCooldown(t *testing.T) {
	ctx, m, r := healthFixture(t)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	scheduled := make(chan error, 1)
	go func() { _, e := m.checkHealth(ctx, r.ID, false, nil); scheduled <- e }()
	<-started
	other := NewManager(m.Repository, m.Config)
	defer other.Close()
	other.OutboundHTTP = m.OutboundHTTP
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			out, e := other.Test(ctx, r.ID)
			if e == nil && !out.Passed {
				e = errors.New("shared result failed")
			}
			errs <- e
		})
	}
	close(release)
	wg.Wait()
	if e := <-scheduled; e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	c, _ := m.Repository.Get(ctx, r.ID)
	stamp := c.Record.LastCheckedAt
	out, e := m.Test(ctx, r.ID)
	if e != nil || !out.Reused || calls.Load() != 1 {
		t.Fatal(out, e, calls.Load())
	}
	fresh, _ := m.Repository.Get(ctx, r.ID)
	if !stamp.Equal(*fresh.Record.LastCheckedAt) {
		t.Fatal("cooldown fabricated new time")
	}
}
func TestHealthDisabledAndStaleRevisionCannotPublish(t *testing.T) {
	ctx, m, r := healthFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return officialReply(req, `{"state":true,"data":{"user_id":"1"}}`), nil
	})}
	pending := make(chan error, 1)
	go func() { _, e := m.checkHealth(ctx, r.ID, false, nil); pending <- e }()
	<-started
	m.Repository.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1,enabled=false,last_checked_at=NULL WHERE id=$1`, r.ID)
	close(release)
	if e := <-pending; !errors.Is(e, ErrRevision) {
		t.Fatal("stale receipt", e)
	}
	got, e := m.Test(ctx, r.ID)
	if e != nil || got.State != "disabled" {
		t.Fatal(got, e)
	}
	c, _ := m.Repository.Get(ctx, r.ID)
	if c.Record.LastCheckedAt != nil {
		t.Fatal("disabled received receipt")
	}
}
func TestHealthTimeoutAndRateLimitPersistWithoutImmediateRetry(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{true: "rate-limit", false: "timeout"}[limited], func(t *testing.T) {
			ctx, m, r := healthFixture(t)
			var calls int
			m.OutboundHTTP = &http.Client{Transport: officialTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if !limited {
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				res := officialReply(req, `{"state":false,"code":40140117}`)
				res.Header.Set("Retry-After", "30000")
				res.StatusCode = 429
				return res, nil
			})}
			check, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
			defer cancel()
			if limited {
				check = ctx
			}
			out := healthTick(t, check, m, r.ID, nil)
			if out.Passed {
				t.Fatal(out)
			}
			if !limited && out.Message != "连接检测超时，请检查网络后重试" {
				t.Fatal(out)
			}
			if limited {
				var delay float64
				m.Repository.DB.QueryRow(ctx, `SELECT extract(epoch FROM(next_check_at-clock_timestamp())) FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&delay)
				if delay < 29990 {
					t.Fatal("ignored Retry-After", delay)
				}
				got, e := m.Test(ctx, r.ID)
				if e != nil || !got.Reused || calls != 1 {
					t.Fatal("manual bypassed rate limit", got, e, calls)
				}
			}
		})
	}
}
func TestHealthPolicyUpgradeShortensHealthyScheduleButNeverFailureCooldown(t *testing.T) {
	ctx, m, r := healthFixture(t)
	m.Repository.DB.Exec(ctx, `UPDATE integrations SET last_check_state='ready',last_checked_at=clock_timestamp()-interval '2 hours' WHERE id=$1`, r.ID)
	m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET failures=0,next_check_at=clock_timestamp()+interval '4 hours' WHERE integration_id=$1`, r.ID)
	if e := m.ScheduleHealth(ctx); e != nil {
		t.Fatal(e)
	}
	var due bool
	m.Repository.DB.QueryRow(ctx, `SELECT next_check_at<=clock_timestamp() FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&due)
	if !due {
		t.Fatal("kept obsolete six-hour success cadence")
	}
	m.Repository.DB.Exec(ctx, `UPDATE integration_health_schedule SET failures=2,next_check_at=clock_timestamp()+interval '6 hours' WHERE integration_id=$1`, r.ID)
	if e := m.ScheduleHealth(ctx); e != nil {
		t.Fatal(e)
	}
	m.Repository.DB.QueryRow(ctx, `SELECT next_check_at>clock_timestamp()+interval '5 hours' FROM integration_health_schedule WHERE integration_id=$1`, r.ID).Scan(&due)
	if !due {
		t.Fatal("restart shortened persisted failure cooldown")
	}
}
