package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"media-tool/internal/platform/database"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func controlFixture(t *testing.T) (context.Context, Repository) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("test DB only")
	}
	return ctx, Repository{DB: db}
}
func controlJob(t *testing.T, r Repository, kind, state string) Job {
	t.Helper()
	ctx := context.Background()
	j, err := r.Create(ctx, kind, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.DB.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, j.ID)
		r.DB.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, j.ID)
	})
	if state == "running" {
		_, err = r.DB.Exec(ctx, `UPDATE jobs SET state='running',lease_token=$2,lease_until=now()+interval '1 minute',locked_by='control-fixture' WHERE id=$1`, j.ID, UUID())
	} else {
		_, err = r.DB.Exec(ctx, `UPDATE jobs SET state=$2 WHERE id=$1`, j.ID, state)
	}
	if err != nil {
		t.Fatal(err)
	}
	j.State = state
	return j
}
func TestGenericTaskPendingCancelAndRetryCAS(t *testing.T) {
	ctx, r := controlFixture(t)
	j := controlJob(t, r, "system.check", "pending")
	out, err := r.Cancel(ctx, j.ID, 1)
	if err != nil || out.State != "cancelled" || out.Queued {
		t.Fatal(out, err)
	}
	if again, err := r.Cancel(ctx, j.ID, 1); err != nil || again.Revision != out.Revision {
		t.Fatal("cancel not idempotent", err)
	}
	var claims int
	if err = r.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE id=$1 AND state IN('pending','retry') AND cancel_requested_at IS NULL`, j.ID).Scan(&claims); err != nil || claims != 0 {
		t.Fatal("cancelled still runnable")
	}
	var verified atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	verify := func(context.Context, Job) (Verification, error) {
		verified.Add(1)
		close(entered)
		<-release
		return Verification{Reason: "confirmed incomplete"}, nil
	}
	done := make(chan error, 1)
	go func() { _, e := r.Retry(ctx, j.ID, out.Revision, verify); done <- e }()
	<-entered
	if _, err = r.Retry(ctx, j.ID, out.Revision, verify); !errors.Is(err, ErrControlConflict) {
		t.Fatal("concurrent retry accepted", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if verified.Load() != 1 {
		t.Fatal("duplicate verification")
	}
	out, err = r.ControlStatus(ctx, j.ID)
	if err != nil || !out.Queued || out.State != "pending" {
		t.Fatal(out, err)
	}
	if _, err = r.Retry(ctx, j.ID, out.Revision, verify); err != nil || verified.Load() != 1 {
		t.Fatal("queued retry duplicated")
	}
}
func TestGenericTaskUnknownMustVerifyFailureAndCompletedConvergence(t *testing.T) {
	ctx, r := controlFixture(t)
	j := controlJob(t, r, "download.add", "unknown")
	out, err := r.Retry(ctx, j.ID, 1, func(context.Context, Job) (Verification, error) {
		return Verification{}, errors.New("owned provider offline")
	})
	if err != nil || out.State != "unknown" || out.Queued {
		t.Fatal(out, err)
	}
	read, err := r.ControlStatus(ctx, j.ID)
	if err != nil || read.Reason != "owned provider offline" {
		t.Fatal("lost reason", read, err)
	}
	out, err = r.Retry(ctx, j.ID, out.Revision, func(context.Context, Job) (Verification, error) {
		return Verification{Complete: true, Reason: "target already exists"}, nil
	})
	if err != nil || out.State != "succeeded" || out.Queued {
		t.Fatal("completed external effect replayed", out, err)
	}
}
func TestGenericRunningCancelContextAcknowledgementAndBoundedWait(t *testing.T) {
	ctx, r := controlFixture(t)
	j := controlJob(t, r, "download.add", "running")
	j.LeaseToken = UUID()
	if _, err := r.DB.Exec(ctx, `UPDATE jobs SET lease_token=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, j.ID, j.LeaseToken); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan struct{})
	w := Worker{Repository: r, Lease: 3 * time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Handlers: map[string]Handler{"download.add": func(ctx context.Context, _ Job) error { close(started); <-ctx.Done(); return ctx.Err() }}}
	go func() { w.execute(ctx, j); close(done) }()
	<-started
	out, err := r.Cancel(ctx, j.ID, 1)
	if err != nil || out.StopPending || out.State != "unknown" {
		t.Fatal("running write cancellation misrepresented", out, err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker context not stopped")
	}
	// Simulate an external call which has not cooperated. HTTP waiting is bounded
	// and no second execution may start before the lease expires/worker exits.
	stuck := controlJob(t, r, "transfer.upload", "running")
	if _, err = r.DB.Exec(ctx, `UPDATE jobs SET lease_token=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, stuck.ID, UUID()); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	out, err = r.Cancel(ctx, stuck.ID, 1)
	if err != nil || !out.StopPending || time.Since(began) > 3*time.Second {
		t.Fatal("unbounded wait/false cancellation", out, err)
	}
	if _, err = r.Retry(ctx, stuck.ID, out.Revision, nil); err == nil {
		t.Fatal("retry raced live external call")
	}
}
func TestGenericCrashLeaseDoesNotReplayAndInspectionSingleOwner(t *testing.T) {
	ctx, r := controlFixture(t)
	j := controlJob(t, r, "download.add", "running")
	if _, err := r.DB.Exec(ctx, `UPDATE jobs SET lease_token=$2,lease_until=now()-interval '1 second' WHERE id=$1`, j.ID, UUID()); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverControlled(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := r.ControlStatus(ctx, j.ID)
	if err != nil || out.State != "unknown" {
		t.Fatal("lease expiry auto replayed", out, err)
	}
	var wg sync.WaitGroup
	var calls atomic.Int32
	for range 8 {
		wg.Go(func() {
			_, _ = r.Retry(ctx, j.ID, out.Revision, func(context.Context, Job) (Verification, error) {
				calls.Add(1)
				return Verification{Complete: true}, nil
			})
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("retry has multiple owners", calls.Load())
	}
}

// These retired kinds have no producers/handlers. Their succeeded history is
// still readable, never claimed/recovered/retried, and never rewritten by I27.
func TestRetiredEmbySucceededHistoryIsUnchanged(t *testing.T) {
	ctx, r := controlFixture(t)
	for _, kind := range []string{"emby.refresh", "emby.confirm", "library.emby.refresh"} {
		t.Run(kind, func(t *testing.T) {
			if NeedsVerification(kind) {
				t.Fatal("retired verification branch remains", kind)
			}
			j := controlJob(t, r, kind, "succeeded")
			snapshot := func() string {
				t.Helper()
				var value string
				if err := r.DB.QueryRow(ctx, `SELECT row_to_json(j)::text FROM jobs j WHERE id=$1`, j.ID).Scan(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			before := snapshot()
			if err := r.recoverControlled(ctx); err != nil {
				t.Fatal(err)
			}
			out, err := r.ControlStatus(ctx, j.ID)
			if err != nil || out.State != "succeeded" || out.Queued {
				t.Fatal(out, err)
			}
			verified := false
			out, err = r.Retry(ctx, j.ID, out.Revision, func(context.Context, Job) (Verification, error) {
				verified = true
				return Verification{}, errors.New("must not verify succeeded history")
			})
			if err != nil || out.State != "succeeded" || verified {
				t.Fatal("history replayed", out, err, verified)
			}
			var runnable bool
			if err = r.DB.QueryRow(ctx, `SELECT state IN('pending','retry') FROM jobs WHERE id=$1`, j.ID).Scan(&runnable); err != nil || runnable {
				t.Fatal("history became runnable", err)
			}
			if after := snapshot(); after != before {
				t.Fatal("terminal historical row changed", kind)
			}
		})
	}
}
