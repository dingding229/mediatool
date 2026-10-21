package migrations_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/probequeue"
)

type pipelineTicket struct{}

func (pipelineTicket) Claim() (string, string, error) {
	return "https://synthetic.invalid/file?SIGN=NEVER_PERSIST", "SyntheticProbe/23B", nil
}

type pipelineLinks struct {
	err   error
	calls int
}

func (l *pipelineLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, nil
}
func (l *pipelineLinks) Fetch(context.Context, string) (probequeue.Ticket, error) {
	l.calls++
	return pipelineTicket{}, l.err
}

type pipelineProbe struct {
	t     *testing.T
	calls int
	err   error
}

func (p *pipelineProbe) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, nil
}
func (p *pipelineProbe) Probe(_ context.Context, url string, o probequeue.ProbeOptions) ([]byte, error) {
	p.calls++
	if url != "https://synthetic.invalid/file?SIGN=NEVER_PERSIST" || o.UserAgent != "SyntheticProbe/23B" {
		p.t.Error("UA/URL pair changed")
	}
	return []byte(`{"format":{"duration":"61.5","filename":"https://synthetic.invalid/file?SIGN=NEVER_PERSIST"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"tags":{"filename":"NEVER_PERSIST"}},{"index":3,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng","title":"https://synthetic.invalid/?SIGN=NEVER_PERSIST"}}]}`), p.err
}
func TestNativeProbeExecutionPersistsWhitelistAndWakeNeverRetries(t *testing.T) {
	f := newProbeQueueFixture(t)
	// These assertions execute the current runtime, not the historical DTOs.
	if _, err := f.provider.UpTo(f.ctx, 73); err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, false)
	links := &pipelineLinks{}
	probe := &pipelineProbe{t: t}
	logs := &bytes.Buffer{}
	engine := probequeue.Engine{Store: f.r, Links: links, Probe: probe, Logger: slog.New(slog.NewJSONHandler(logs, nil))}
	wake := jobs.Repository{DB: f.pool}
	run := func() {
		t.Helper()
		j, err := wake.ClaimQueue(f.ctx, "synthetic-probe", time.Minute, "probe")
		if err != nil || j == nil {
			t.Fatal(err)
		}
		if err := engine.Process(f.ctx, *j); err != nil {
			t.Fatal("probe failure reached generic job retry", err)
		}
		if err := wake.Finish(f.ctx, *j, nil); err != nil {
			t.Fatal(err)
		}
		if f.text(t, `SELECT state FROM jobs WHERE id=$1`, j.ID) != "succeeded" {
			t.Fatal("wake not succeeded")
		}
	}
	run()
	snapshot, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || snapshot == nil || snapshot.Result.DurationTicks != 615000000 {
		t.Fatal("duration snapshot", err)
	}
	body := f.text(t, `SELECT result::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file)
	if strings.Contains(body, "NEVER_PERSIST") || strings.Contains(body, "filename") {
		t.Fatal("raw JSON persisted")
	}
	if snapshot.Result.SchemaVersion != 1 || len(snapshot.Result.Streams) != 2 || snapshot.Result.Streams[1].Index != 3 || snapshot.Result.Streams[1].Title != "" {
		t.Fatal("full typed snapshot missing")
	}
	f.enqueue(t, true)
	links.err = &linksource.Failure{Class: linksource.Retryable, Reason: linksource.Limited, RetryAfter: 5 * time.Minute}
	run()
	if f.state(t) != "pending" {
		t.Fatal("retry state")
	}
	if _, count, err := f.r.ScheduleBatch(f.ctx, ""); err != nil || count != 0 {
		t.Fatal("sweep bypassed backoff", count, err)
	}
	old, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || old.Result.DurationTicks != snapshot.Result.DurationTicks {
		t.Fatal("failed probe erased success")
	}

	if f.text(t, `SELECT result::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != body {
		t.Fatal("failed probe changed typed snapshot")
	}
	f.due(t)
	if _, count, err := f.r.ScheduleBatch(f.ctx, ""); err != nil || count != 1 {
		t.Fatal("due retry not woken", count, err)
	}
	links.err = &linksource.Failure{Class: linksource.Manual, Reason: linksource.Authorization}
	run()
	if f.state(t) != "needs_action" {
		t.Fatal("manual state")
	}
	f.enqueue(t, true)
	links.err = nil
	probe.err = errors.New("https://synthetic.invalid/?SIGN=NEVER_PERSIST")
	run()
	if f.state(t) != "pending" || strings.Contains(logs.String(), "NEVER_PERSIST") || strings.Contains(logs.String(), "https://") {
		t.Fatal("unsafe failure logging")
	}
}
func TestNativeProbeDefaultOffBothSwitchesGateEnqueueAndExecution(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.exec(t, `UPDATE native_server_settings SET enabled=false WHERE singleton`)
	if f.text(t, `SELECT probe_enabled::text FROM native_server_settings WHERE singleton`) != "true" {
		t.Fatal("probe switch default")
	}
	for _, settings := range []string{`enabled=false,probe_enabled=true`, `enabled=true,probe_enabled=false`} {
		f.exec(t, `UPDATE native_server_settings SET `+settings+` WHERE singleton`)
		if queued, err := f.r.Enqueue(f.ctx, f.a.asset); queued || !errors.Is(err, probequeue.ErrDisabled) {
			t.Fatal("disabled enqueue", err)
		}
		if _, count, err := f.r.ScheduleBatch(f.ctx, ""); err != nil || count != 0 {
			t.Fatal("disabled sweep")
		}
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 0 {
		t.Fatal("default off created work")
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true,probe_enabled=true WHERE singleton`)
	f.enqueue(t, false)
	f.exec(t, `UPDATE native_server_settings SET probe_enabled=false WHERE singleton`)
	links := &pipelineLinks{}
	probe := &pipelineProbe{t: t}
	raw := []byte(`{"assetId":"` + f.a.asset + `"}`)
	if err := (probequeue.Engine{Store: f.r, Links: links, Probe: probe}).Process(f.ctx, jobs.Job{Payload: raw}); err != nil {
		t.Fatal(err)
	}
	if links.calls != 0 || probe.calls != 0 || f.state(t) != "pending" {
		t.Fatal("disabled work executed")
	}
	if l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute); err != nil || l != nil {
		t.Fatal("disabled lease claimed")
	}
	f.exec(t, `UPDATE native_server_settings SET probe_enabled=true WHERE singleton`)
	lease := f.claim(t)
	f.exec(t, `UPDATE native_server_settings SET probe_enabled=false WHERE singleton`)
	f.succeed(t, lease, 999, false)
	if snapshot, err := f.r.Current(f.ctx, f.a.file); err != nil || snapshot != nil {
		t.Fatal("disabled completion installed a snapshot", err)
	}

}
func TestNativeProbeSweep200CursorFingerprintAndIdempotence(t *testing.T) {
	f := newProbeQueueFixture(t)
	media := f.media(t, 230063, "Synthetic sweep", "2026", `{}`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)SELECT md5('sweep-file:'||n)::uuid,$1,'/115/sweep/'||n,1 FROM generate_series(1,250)n`, f.storage)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id)SELECT md5('sweep-asset:'||n)::uuid,md5('sweep-file:'||n)::uuid FROM generate_series(1,250)n`)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state)SELECT md5('sweep-asset:'||n)::uuid,$1,'ready' FROM generate_series(1,250)n`, media)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT md5('sweep-strm:'||n)::uuid,md5('sweep-asset:'||n)::uuid,'Synthetic/'||n||'.strm','synthetic' FROM generate_series(1,250)n`)
	cursor, count, err := f.r.ScheduleBatch(f.ctx, "")
	if err != nil || count != 200 {
		t.Fatal("batch bound", count, err)
	}
	_, count, err = f.r.ScheduleBatch(f.ctx, cursor)
	if err != nil || count != 51 {
		t.Fatal("resume cursor", count, err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 251 {
		t.Fatal("missing wakes")
	}
	if _, _, err := f.r.ScheduleBatch(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 251 {
		t.Fatal("duplicate delivery accumulated wakes")
	}
	l := f.claim(t)
	f.succeed(t, l, 1, true)
	f.exec(t, `UPDATE cloud_files SET metadata='{"2":"replacement-sha"}' WHERE id=$1`, f.a.file)
	if _, _, err := f.r.ScheduleBatch(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	// UUID ordering may put this asset beyond the first page; resume once.
	next, _, err := f.r.ScheduleBatch(f.ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.r.ScheduleBatch(f.ctx, next); err != nil {
		t.Fatal(err)
	}
	if f.state(t) != "pending" {
		t.Fatal("changed fingerprint not scheduled")
	}
}
func TestNativeProbeUppercaseIDsUseOneBusinessKey(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	if _, err := f.r.Enqueue(f.ctx, strings.ToUpper(f.a.asset)); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 1 {
		t.Fatal("UUID casing duplicated key")
	}
}

type probeAdminSession struct{}

func (probeAdminSession) Session(context.Context, []byte) (accounts.Account, error) {
	return accounts.Account{ID: "synthetic-admin", Username: "synthetic_admin"}, nil
}
func TestNativeProbeAdminHTTPRetryAuditsAndEnforcesCSRF(t *testing.T) {
	f := newProbeQueueFixture(t)
	cfg := config.Config{PublicURL: "https://synthetic-admin.invalid", SessionKey: strings.Repeat("a", 64), STRMKey: strings.Repeat("b", 64), STRMAllowedRoots: []string{"/115"}}
	handler := httpapi.New(cfg, httpapi.Dependencies{DB: f.pool, AuthSessions: probeAdminSession{}, Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	token, err := (auth.Auth{Key: cfg.SessionKey}).Issue(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/media-server/native/probe/" + f.a.asset + "/retry"
	call := func(cookie, csrf bool) int {
		r := httptest.NewRequest("POST", path, nil)
		if cookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if status := call(false, true); status != 401 {
		t.Fatal("anonymous", status)
	}
	if status := call(true, false); status != 403 {
		t.Fatal("missing CSRF", status)
	}
	if status := call(true, true); status != 202 {
		t.Fatal("admin retry", status)
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.probe.retry'`) != 1 {
		t.Fatal("admin retry missing audit")
	}
	f.exec(t, `UPDATE native_server_settings SET probe_enabled=false WHERE singleton`)
	if status := call(true, true); status != 503 {
		t.Fatal("disabled admin retry", status)
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.probe.retry'`) != 1 {
		t.Fatal("disabled retry audited as success")
	}
	media, err := httpapi.NativeMediaHandler(cfg, httpapi.Dependencies{DB: f.pool})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	media.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
	if w.Code != 404 {
		t.Fatal("management retry on media port", w.Code)
	}
}
