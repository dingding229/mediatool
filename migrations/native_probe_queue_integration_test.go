package migrations_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/probequeue"
)

type probeQueueFixture struct {
	movieFixture
	a movieAsset
	r probequeue.Repository
}

func newProbeQueueFixture(t *testing.T) probeQueueFixture {
	t.Helper()
	f := newMovieFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 59); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true WHERE singleton`)
	media := f.media(t, 230059, "Synthetic probe queue", "2026", `{}`)
	return probeQueueFixture{f, f.asset(t, media, "g:test-a", time.Now(), "mkv"), probequeue.Repository{DB: f.pool}}
}
func (f probeQueueFixture) enqueue(t *testing.T, force bool) {
	t.Helper()
	var queued bool
	var err error
	if force {
		queued, err = f.r.Retry(f.ctx, f.a.asset)
	} else {
		queued, err = f.r.Enqueue(f.ctx, f.a.asset)
	}
	if err != nil || !queued {
		t.Fatal("enqueue", queued, err)
	}
}
func (f probeQueueFixture) claim(t *testing.T) *probequeue.Lease {
	t.Helper()
	l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute)
	if err != nil || l == nil {
		t.Fatal("claim", l, err)
	}
	return l
}
func (f probeQueueFixture) state(t *testing.T) string {
	t.Helper()
	return f.text(t, `SELECT state FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file)
}
func (f probeQueueFixture) due(t *testing.T) {
	t.Helper()
	f.exec(t, `UPDATE media_probe_results SET next_attempt_at=now()-interval '1 second' WHERE cloud_file_id=$1`, f.a.file)
}
func assertProbeCommit(t *testing.T, ok bool, err error, want bool) {
	t.Helper()
	if err != nil || ok != want {
		t.Fatalf("commit=%v want=%v err=%v", ok, want, err)
	}
}
func (f probeQueueFixture) succeed(t *testing.T, l *probequeue.Lease, ticks int64, want bool) {
	t.Helper()
	ok, err := f.r.Succeed(f.ctx, *l, probequeue.Result{SchemaVersion: 1, DurationTicks: ticks})
	assertProbeCommit(t, ok, err, want)
}
func (f probeQueueFixture) fail(t *testing.T, l *probequeue.Lease, class linksource.Class, reason string, want bool) {
	t.Helper()
	ok, err := f.r.Fail(f.ctx, *l, probequeue.Failure{Class: class, Reason: reason})
	assertProbeCommit(t, ok, err, want)
}

func TestNativeProbeMigrationConstraintsDownUp(t *testing.T) {
	f := newMovieFixture(t)
	media := f.media(t, 230060, "Synthetic upgrade", "2026", `{}`)
	a := f.asset(t, media, "g:test-a", time.Now(), "mkv")
	b := f.asset(t, media, "g:test-a", time.Now(), "mkv")
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,fingerprint,state,result,probed_at) VALUES($1,'old-success','ready','{"durationTicks":77}',now())`, a.file)
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,fingerprint,state,probing_started_at,error) VALUES($1,'old-probing','probing',now(),'private upstream message')`, b.file)
	if _, err := f.provider.UpTo(f.ctx, 59); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT result_fingerprint FROM media_probe_results WHERE cloud_file_id=$1`, a.file) != "old-success" {
		t.Fatal("lost previous result")
	}
	if f.text(t, `SELECT state FROM media_probe_results WHERE cloud_file_id=$1`, b.file) != "pending" {
		t.Fatal("old attempt not recovered")
	}
	if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='media_probe_results' AND column_name='error'`) != 0 {
		t.Fatal("free-form errors retained")
	}
	for _, tt := range []struct{ name, sql string }{
		{"state", `UPDATE media_probe_results SET state='bogus' WHERE cloud_file_id=$1`},
		{"fingerprint", `UPDATE media_probe_results SET file_fingerprint='' WHERE cloud_file_id=$1`},
		{"class", `UPDATE media_probe_results SET last_error_class='upstream text',last_error_reason='timeout' WHERE cloud_file_id=$1`},
		{"reason", `UPDATE media_probe_results SET last_error_class='manual',last_error_reason='https://synthetic.example/?token=private' WHERE cloud_file_id=$1`},
		{"paired errors", `UPDATE media_probe_results SET last_error_class='retryable',last_error_reason=NULL WHERE cloud_file_id=$1`},
		{"missing lease", `UPDATE media_probe_results SET state='probing',probing_started_at=now() WHERE cloud_file_id=$1`},
		{"orphan lease", `UPDATE media_probe_results SET lease_token='11111111-1111-4111-8111-111111111111' WHERE cloud_file_id=$1`},
		{"manual state", `UPDATE media_probe_results SET state='needs_action',last_error_class='permanent',last_error_reason='file_missing' WHERE cloud_file_id=$1`},
		{"failed class", `UPDATE media_probe_results SET state='failed',last_error_class=NULL,last_error_reason=NULL WHERE cloud_file_id=$1`},
		{"pending class", `UPDATE media_probe_results SET state='pending',last_error_class='manual',last_error_reason='authorization' WHERE cloud_file_id=$1`},
		{"ready fingerprint", `UPDATE media_probe_results SET file_fingerprint='new' WHERE cloud_file_id=$1`},
		{"snapshot pair", `UPDATE media_probe_results SET result_fingerprint=NULL WHERE cloud_file_id=$1`},
	} {
		t.Run(tt.name, func(t *testing.T) { f.rejects(t, "23514", tt.sql, a.file) })
	}
	f.exec(t, `UPDATE media_probe_results SET state='needs_action',last_error_class='manual',last_error_reason='authorization' WHERE cloud_file_id=$1`, b.file)
	if _, err := f.provider.DownTo(f.ctx, 58); err != nil {
		t.Fatal("down", err)
	}
	if f.text(t, `SELECT fingerprint FROM media_probe_results WHERE cloud_file_id=$1`, a.file) != "old-success" || f.text(t, `SELECT state FROM media_probe_results WHERE cloud_file_id=$1`, b.file) != "failed" {
		t.Fatal("down lost cache or terminal state")
	}
	f.rejects(t, "23514", `UPDATE media_probe_results SET state='needs_action' WHERE cloud_file_id=$1`, b.file)
	if _, err := f.provider.UpTo(f.ctx, 59); err != nil {
		t.Fatal("reup", err)
	}
	if f.text(t, `SELECT result->>'durationTicks' FROM media_probe_results WHERE cloud_file_id=$1`, a.file) != "77" {
		t.Fatal("up/down/up lost success")
	}
}
func TestNativeProbeQueueIdempotenceAndSingleSchedulingAuthority(t *testing.T) {
	f := newProbeQueueFixture(t)
	for range 3 {
		f.enqueue(t, false)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 1 || f.integer(t, `SELECT count(*) FROM media_probe_results`) != 1 {
		t.Fatal("duplicate work")
	}
	fp := f.text(t, `SELECT file_fingerprint FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file)
	if len(fp) != 64 || f.text(t, `SELECT idempotency_key FROM jobs WHERE kind='native.probe'`) != "probe:"+f.a.asset+":"+fp {
		t.Fatal("non-deterministic key")
	}
	if f.integer(t, `SELECT max_attempts FROM jobs WHERE kind='native.probe'`) != 1 {
		t.Fatal("generic retry enabled")
	}
	// An obsolete/early job cannot override the probe table's backoff.
	f.exec(t, `UPDATE media_probe_results SET next_attempt_at=now()+interval '1 hour' WHERE cloud_file_id=$1`, f.a.file)
	queued, err := f.r.Enqueue(f.ctx, f.a.asset)
	if err != nil || queued {
		t.Fatal("enqueue ignored probe backoff")
	}
	if l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute); err != nil || l != nil {
		t.Fatal("early job acquired probe lease")
	}
	// Conversely, jobs.available_at does not prevent a due probe claim.
	f.due(t)
	f.exec(t, `UPDATE jobs SET available_at=now()+interval '1 day' WHERE kind='native.probe'`)
	lease := f.claim(t)
	if lease.Attempt != 1 {
		t.Fatal("job attempt leaked into probe attempts")
	}
	f.succeed(t, lease, 42, true)
	queued, err = f.r.Enqueue(f.ctx, f.a.asset)
	if err != nil || queued || f.state(t) != "ready" {
		t.Fatal("unchanged success was queued")
	}
}
func TestNativeProbeFailurePathsAndManualRecovery(t *testing.T) {
	f := newProbeQueueFixture(t)
	for _, tt := range []struct {
		class         linksource.Class
		reason, state string
	}{{linksource.Permanent, "file_missing", "failed"}, {linksource.Manual, "authorization", "needs_action"}, {linksource.Retryable, "timeout", "pending"}} {
		f.enqueue(t, true)
		l := f.claim(t)
		f.fail(t, l, tt.class, tt.reason, true)
		if f.state(t) != tt.state {
			t.Fatal("wrong transition", f.state(t))
		}
		if queued, err := f.r.Enqueue(f.ctx, f.a.asset); err != nil || queued {
			t.Fatal("terminal/backoff auto reset")
		}
		if l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute); err != nil || l != nil {
			t.Fatal("terminal/backoff claimed")
		}
		if queued, err := f.r.Retry(f.ctx, f.a.asset); err != nil || !queued || f.state(t) != "pending" {
			t.Fatal("manual retry")
		}
		if f.integer(t, `SELECT attempts FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != 0 {
			t.Fatal("manual attempts not reset")
		}
	}
	l := f.claim(t)
	ok, err := f.r.Fail(f.ctx, *l, probequeue.Failure{Class: linksource.Manual, Reason: "https://synthetic.example/?token=private"})
	if ok || !errors.Is(err, probequeue.ErrInvalid) || f.state(t) != "probing" {
		t.Fatal("untrusted error accepted")
	}
	ok, err = f.r.Succeed(f.ctx, *l, probequeue.Result{SchemaVersion: 1, DurationTicks: -1})
	if ok || !errors.Is(err, probequeue.ErrInvalid) {
		t.Fatal("negative ticks accepted")
	}

	for _, invalid := range []probequeue.Result{{DurationTicks: 1}, {SchemaVersion: 1, Container: "https://synthetic.invalid/private"}} {
		ok, err = f.r.Succeed(f.ctx, *l, invalid)
		if ok || !errors.Is(err, probequeue.ErrInvalid) || f.state(t) != "probing" {
			t.Fatal("unversioned/unsafe result persisted")
		}
	}
}
func TestNativeProbeRetryScheduleAndSuggestedDelay(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	for i, want := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		l := f.claim(t)
		if l.Attempt != i+1 {
			t.Fatal("attempt not persisted")
		}
		f.fail(t, l, linksource.Retryable, string(probequeue.ProcessFailed), true)
		var seconds float64
		if err := f.pool.QueryRow(f.ctx, `SELECT extract(epoch FROM next_attempt_at-clock_timestamp())::float8 FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file).Scan(&seconds); err != nil {
			t.Fatal(err)
		}
		if seconds > want.Seconds()+1 || seconds < want.Seconds()-2 {
			t.Fatal("wrong backoff", seconds, want)
		}
		f.due(t)
	}
	l := f.claim(t)
	ok, err := f.r.Fail(f.ctx, *l, probequeue.Failure{Class: linksource.Retryable, Reason: string(linksource.Limited), RetryAfter: 17 * time.Minute})
	assertProbeCommit(t, ok, err, true)
	var seconds int
	if err := f.pool.QueryRow(f.ctx, `SELECT extract(epoch FROM next_attempt_at-clock_timestamp())::int FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file).Scan(&seconds); err != nil || seconds < 1018 || seconds > 1020 {
		t.Fatal("upstream retry interval lost", seconds, err)
	}
}
func TestNativeProbeLeaseRecoveryAndLateWorkerFence(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	initial := f.claim(t)
	f.succeed(t, initial, 41, true)
	previous, err := f.r.Current(f.ctx, f.a.file)
	if err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, true)
	old := f.claim(t)
	f.exec(t, `UPDATE media_probe_results SET probing_started_at=now()-interval '3 minute',lease_until=now()-interval '1 minute' WHERE cloud_file_id=$1`, f.a.file)
	f.succeed(t, old, 999, false)
	f.fail(t, old, linksource.Permanent, "file_missing", false)
	if n, err := f.r.RecoverExpired(f.ctx); err != nil || n != 1 {
		t.Fatal("lease recovery", n, err)
	}
	after, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || !reflect.DeepEqual(previous, after) {
		t.Fatal("lease recovery erased success", err)
	}
	fresh := f.claim(t)
	if fresh.Token == old.Token || fresh.Attempt != old.Attempt+1 {
		t.Fatal("fence not renewed")
	}
	f.succeed(t, old, 999, false)
	f.fail(t, old, linksource.Manual, "authorization", false)
	forged := *fresh
	forged.Attempt++
	f.succeed(t, &forged, 999, false)
	f.succeed(t, fresh, 123, true)
	if n, err := f.r.RecoverExpired(f.ctx); err != nil || n != 0 {
		t.Fatal("repeated recovery not idempotent")
	}
}
func TestNativeProbeConcurrentClaimsAndSkipLocked(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(f.ctx, `SELECT cloud_file_id FROM media_probe_results WHERE cloud_file_id=$1 FOR UPDATE`, f.a.file); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 500*time.Millisecond)
	defer cancel()
	if l, err := f.r.Claim(ctx, f.a.asset, time.Minute); err != nil || l != nil {
		t.Fatal("claim blocked instead of SKIP LOCKED", err)
	}
	if err := tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	result := make(chan *probequeue.Lease, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute); result <- l; errs <- err })
	}
	wg.Wait()
	close(result)
	close(errs)
	claimed := 0
	for l := range result {
		if l != nil {
			claimed++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if claimed != 1 {
		t.Fatal("duplicate concurrent claim", claimed)
	}
}
func TestNativeProbeFingerprintAndFailedReplacementPreserveSuccess(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	a := f.claim(t)
	f.succeed(t, a, 100, true)
	original, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || original == nil {
		t.Fatal(err)
	}
	f.enqueue(t, true)
	inflight := f.claim(t)
	// Even before enqueue/recovery, current cloud evidence fences late success.
	f.exec(t, `UPDATE cloud_files SET metadata='{"2":"new-sha1"}' WHERE id=$1`, f.a.file)
	f.succeed(t, inflight, 999, false)
	f.enqueue(t, false)
	f.succeed(t, inflight, 999, false)
	f.fail(t, inflight, linksource.Permanent, "file_missing", false)
	b := f.claim(t)
	if a.Fingerprint == b.Fingerprint || b.Attempt != 1 {
		t.Fatal("new fingerprint not reset")
	}
	for _, outcome := range []linksource.Class{linksource.Retryable, linksource.Permanent, linksource.Manual} {
		f.fail(t, b, outcome, "unavailable", true)
		snapshot, err := f.r.Current(f.ctx, f.a.file)
		if err != nil || !reflect.DeepEqual(snapshot, original) {
			t.Fatal("failure erased/replaced last success", err)
		}
		f.enqueue(t, true)
		b = f.claim(t)
	}
	// Manual retry on the SAME fingerprint invalidates the old lease too.
	f.enqueue(t, true)
	newer := f.claim(t)
	f.succeed(t, b, 999, false)
	f.succeed(t, newer, 200, true)
	current, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || current.Result.DurationTicks != 200 || current.Fingerprint != newer.Fingerprint {
		t.Fatal("fresh success not installed", err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 2 {
		t.Fatal("duplicate wakes for same asset fingerprint")
	}
}
func TestNativeProbeFingerprintIsStableAndUsesPersistedHashMap(t *testing.T) {
	f := newProbeQueueFixture(t)
	fingerprint := func() string {
		return f.text(t, `SELECT native_probe_fingerprint(size_bytes,provider_file_id,metadata) FROM cloud_files WHERE id=$1`, f.a.file)
	}
	first := fingerprint()
	f.exec(t, `UPDATE cloud_files SET updated_at=now(),full_path='/115/renamed.mkv' WHERE id=$1`, f.a.file)
	if fingerprint() != first {
		t.Fatal("scan timestamp or path churn invalidates probe")
	}
	for i, change := range []string{`size_bytes=size_bytes+1`, `provider_file_id='replacement-id'`, `metadata='{"2":"SHA1_A"}'`, `metadata='{"2":"SHA1_B"}'`, `metadata='{"2":"SHA1_B","mtime":"2026-10-16T00:00:00Z"}'`} {
		before := fingerprint()
		f.exec(t, `UPDATE cloud_files SET `+change+` WHERE id=$1`, f.a.file)
		if fingerprint() == before {
			t.Fatal("stable content field missed", i)
		}
	}
	before := fingerprint()
	f.exec(t, `UPDATE cloud_files SET metadata='{"mtime":"2026-10-16T00:00:00Z","2":"sha1_b"}' WHERE id=$1`, f.a.file)
	if fingerprint() != before {
		t.Fatal("hash casing/JSON key order changed fingerprint")
	}
}
func TestNativeProbeUnexposableAssetsAndWakeCompletion(t *testing.T) {
	f := newProbeQueueFixture(t)
	// These assertions execute the current runtime, not the historical DTOs.
	if _, err := f.provider.UpTo(f.ctx, 73); err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, false)
	jobRepo := jobs.Repository{DB: f.pool}
	j, err := jobRepo.ClaimQueue(f.ctx, "synthetic-probe", time.Minute, "probe")
	if err != nil || j == nil {
		t.Fatal("wake claim", err)
	}
	l := f.claim(t)
	f.fail(t, l, linksource.Retryable, "timeout", true)
	// Failed PROBE -> successful wake, without job-level retry/backoff.
	if err := jobRepo.Finish(f.ctx, *j, nil); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT state FROM jobs WHERE id=$1`, j.ID) != "succeeded" {
		t.Fatal("wake retried probe failure")
	}
	queued, err := f.r.Enqueue(f.ctx, f.a.asset)
	if err != nil || queued {
		t.Fatal("wake bypassed backoff")
	}
	f.due(t)
	f.enqueue(t, false)
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 1 {
		t.Fatal("duplicate wake")
	}
	j, err = jobRepo.ClaimQueue(f.ctx, "synthetic-probe", time.Minute, "probe")
	if err != nil || j == nil {
		t.Fatal(err)
	}
	f.enqueue(t, true)
	if err := jobRepo.Finish(f.ctx, *j, nil); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT state FROM jobs WHERE id=$1`, j.ID) != "pending" {
		t.Fatal("running wake dropped rerun")
	}
	l = f.claim(t)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, f.a.file)
	f.succeed(t, l, 999, false)
	f.fail(t, l, linksource.Permanent, "not_exposable", true)
	if _, err := f.r.Retry(f.ctx, f.a.asset); !errors.Is(err, probequeue.ErrNotExposable) {
		t.Fatal("missing asset manually exposed", err)
	}
	if l, err := f.r.Claim(f.ctx, f.a.asset, time.Minute); err != nil || l != nil {
		t.Fatal("missing asset claimed")
	}
}
func TestNativeProbeRecoveryBatchIsBoundedAndInterruptible(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes) SELECT md5('recover:'||n::text)::uuid,$1,'/115/recover/'||n::text,1 FROM generate_series(1,205)n`, f.storage)
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,state,attempts,probing_started_at,lease_until,lease_token)
 SELECT id,repeat('b',64),'probing',1,now()-interval '3 minute',now()-interval '1 minute',id FROM cloud_files WHERE full_path LIKE '/115/recover/%'`)
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.r.RecoverExpired(ctx); err == nil {
		t.Fatal("canceled recovery ignored")
	}
	for _, want := range []int64{200, 5, 0} {
		got, err := f.r.RecoverExpired(f.ctx)
		if err != nil || got != want {
			t.Fatal("batch recovery", got, want, err)
		}
	}
	if count := f.integer(t, `SELECT count(*) FROM media_probe_results WHERE state='pending' AND lease_token IS NULL AND last_error_reason='lease_expired'`); count != 205 {
		t.Fatal(fmt.Sprint("lost expired attempts: ", count))
	}
	if f.integer(t, `SELECT count(*) FROM media_probe_results WHERE result<>'{}' OR result_fingerprint IS NOT NULL`) != 0 {
		t.Fatal("recovery fabricated success")
	}
}
func TestNativeProbeConcurrentEnqueueDeduplicates(t *testing.T) {
	f := newProbeQueueFixture(t)
	errs := make(chan error, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _, err := f.r.Enqueue(f.ctx, f.a.asset); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 1 || f.integer(t, `SELECT count(*) FROM media_probe_results`) != 1 {
		t.Fatal("concurrent enqueue duplicated work")
	}
	if !strings.HasPrefix(f.text(t, `SELECT idempotency_key FROM jobs WHERE kind='native.probe'`), "probe:") {
		t.Fatal("wrong identity")
	}
}

func TestNativeProbeManualRetryAuditAndLeaseInvalidation(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	old := f.claim(t)
	before := f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.probe.retry'`)
	// DDL correctly reports zero affected rows; do not use the DML fixture helper.
	if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE audit_logs RENAME TO synthetic_audit_unavailable`); err != nil {
		t.Fatal(err)
	}
	queued, err := f.r.Retry(f.ctx, f.a.asset)
	if err == nil || queued || f.state(t) != "probing" {
		t.Fatal("audit failure did not roll back retry")
	}
	if f.text(t, `SELECT lease_token::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != old.Token {
		t.Fatal("failed retry invalidated lease")
	}
	if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE synthetic_audit_unavailable RENAME TO audit_logs`); err != nil {
		t.Fatal(err)
	}
	queued, err = f.r.Retry(f.ctx, f.a.asset)
	if err != nil || !queued {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.probe.retry'`) != before+1 {
		t.Fatal("manual retry not audited")
	}
	details := f.text(t, `SELECT details::text FROM audit_logs WHERE action='native.probe.retry' ORDER BY id DESC LIMIT 1`)
	if !strings.Contains(details, f.a.asset) || strings.Contains(details, "http") || strings.Contains(details, "token") {
		t.Fatal("unsafe audit details")
	}
	f.succeed(t, old, 999, false)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, f.a.file)
	if _, err := f.r.Retry(f.ctx, f.a.asset); !errors.Is(err, probequeue.ErrNotExposable) {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.probe.retry'`) != before+1 {
		t.Fatal("rejected retry wrote success audit")
	}
}

func TestNativeProbeMigrationNormalizesDirtyLegacyReceipts(t *testing.T) {
	f := newMovieFixture(t)
	media := f.media(t, 230061, "Synthetic dirty cache", "2026", `{}`)
	// Model an imported/NOT VALID legacy table: normal 00055 already rejects bad
	// ready receipts, but upgrade must also tolerate data loaded before validation.
	var constraints []struct{ Name, Definition string }
	rows, err := f.db.QueryContext(f.ctx, `SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='media_probe_results'::regclass AND contype='c' AND ((pg_get_constraintdef(oid) LIKE '%ready%' AND pg_get_constraintdef(oid) LIKE '%probed_at%') OR pg_get_constraintdef(oid) LIKE '%probing_started_at%')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c struct{ Name, Definition string }
		if err := rows.Scan(&c.Name, &c.Definition); err != nil {
			t.Fatal(err)
		}
		constraints = append(constraints, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range constraints {
		if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE media_probe_results DROP CONSTRAINT `+pgx.Identifier{c.Name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		state, result  string
		receipt, start bool
		want           string
		keep           bool
	}{
		{"ready", `{}`, true, false, "pending", false}, {"ready", `{"durationTicks":1}`, false, false, "pending", false},
		{"ready", `{}`, false, false, "pending", false}, {"failed", `{}`, true, true, "failed", false},
		{"failed", `{"durationTicks":2}`, false, false, "failed", false}, {"failed", `{"durationTicks":3}`, true, false, "failed", true},
		{"pending", `{"durationTicks":4}`, true, true, "pending", true}, {"pending", `{}`, true, false, "pending", false},
		{"probing", `{"durationTicks":5}`, true, true, "pending", true},
	}
	files := []string{}
	for _, c := range cases {
		a := f.asset(t, media, "g:test-a", time.Now(), "mkv")
		files = append(files, a.file)
		f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,fingerprint,state,result,probed_at,probing_started_at,error,next_attempt_at)VALUES($1,'legacy',$2,$3,CASE WHEN $4 THEN now() END,CASE WHEN $5 THEN now() END,'SYNTHETIC_PRIVATE_ERROR',now()+interval '1 day')`, a.file, c.state, c.result, c.receipt, c.start)
	}
	for _, c := range constraints {
		if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE media_probe_results ADD CONSTRAINT `+pgx.Identifier{c.Name}.Sanitize()+` `+c.Definition+` NOT VALID`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.provider.UpTo(f.ctx, 59); err != nil {
		t.Fatal("dirty upgrade failed", err)
	}
	for i, c := range cases {
		var state string
		var retained, due, clean bool
		if err := f.db.QueryRowContext(f.ctx, `SELECT state,result_fingerprint IS NOT NULL,next_attempt_at<=now(),probing_started_at IS NULL AND lease_token IS NULL AND lease_until IS NULL FROM media_probe_results WHERE cloud_file_id=$1`, files[i]).Scan(&state, &retained, &due, &clean); err != nil {
			t.Fatal(err)
		}
		if state != c.want || retained != c.keep || !clean {
			t.Fatal("dirty normalization", i, state, retained, clean)
		}
		if (c.state == "ready" || c.state == "probing") && !due {
			t.Fatal("damaged receipt not made due", i)
		}
	}
	if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='media_probe_results' AND column_name='error'`) != 0 {
		t.Fatal("residual raw errors")
	}
}
