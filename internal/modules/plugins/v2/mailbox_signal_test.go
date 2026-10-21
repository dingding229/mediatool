package v2

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
)

func TestMailboxCommitSignalsAndLaneCleanup(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	_, d, e := s.Descriptor(ctx, l.ID, "api", "admin")
	if e != nil {
		t.Fatal(e)
	}
	lane := &mailboxLane{wake: make(chan struct{}, 1), changed: make(chan struct{})}
	key := mailboxKey{s.DB, l.ID}
	mailboxLanes.Store(key, lane)
	defer mailboxLanes.Delete(key)
	run, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), jobs.UUID(), "", nil, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-lane.wake:
	default:
		t.Fatal("committed invocation did not wake lane")
	}
	var committed bool
	if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs WHERE id=$1)`, run).Scan(&committed); e != nil || !committed {
		t.Fatal("wake before commit", e)
	}
	if _, e = s.Submit(ctx, snap, d, json.RawMessage(`{}`), "", "", nil, time.Now().Add(time.Minute)); e == nil {
		t.Fatal("invalid submit accepted")
	}
	select {
	case <-lane.wake:
		t.Fatal("failed submit woke lane")
	default:
	}
	mailboxLanes.CompareAndDelete(key, lane)
	if s.mailbox(l.ID) != nil {
		t.Fatal("exited lane retained")
	}
	// Hints for an exited lane are safe, and never create cache entries.
	s.mailbox(l.ID).queued()
	s.mailbox(l.ID).completed()
}

func TestMailboxCompletionBroadcastAndCoalescing(t *testing.T) {
	lane := &mailboxLane{wake: make(chan struct{}, 1), changed: make(chan struct{})}
	for range 50 {
		var wg sync.WaitGroup
		observed := lane.observed()
		for range 16 {
			wg.Go(func() { <-observed })
		}
		lane.completed()
		wg.Wait()
		select {
		case <-lane.observed():
			t.Fatal("new observer saw stale completion")
		default:
		}
		for range 16 {
			wg.Go(lane.queued)
		}
		wg.Wait()
		<-lane.wake
		select {
		case <-lane.wake:
			t.Fatal("unbounded queue hints")
		default:
		}
	}
}

func TestMailboxDeadlineCompletionRaceKeepsReceiptAndFences(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	_, d, e := s.Descriptor(ctx, l.ID, "api", "admin")
	if e != nil {
		t.Fatal(e)
	}
	run, e := s.Submit(ctx, snap, d, json.RawMessage(`{}`), jobs.UUID(), "", nil, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	token := jobs.UUID()
	// Deterministically interleave caller deadline convergence before RPC completion.
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='unknown',reason='deadline',lease_token=$2,lease_until=now()+interval '30 seconds',finished_at=now() WHERE id=$1`, run, token); e != nil {
		t.Fatal(e)
	}
	if e = s.finishCall(ctx, run, token, RunSucceeded, RunNoReason, []byte("must-not-be-persisted")); e != nil {
		t.Fatal("cooperative completion killed live lane", e)
	}
	var state, reason string
	var cipher []byte
	if e = s.DB.QueryRow(ctx, `SELECT state,reason,result_ciphertext FROM plugin_v2_runs WHERE id=$1`, run).Scan(&state, &reason, &cipher); e != nil || state != "unknown" || reason != "deadline" || len(cipher) != 0 {
		t.Fatal("terminal receipt overwritten", state, reason, e)
	}
	if e = s.finishCall(ctx, run, token, RunUnknown, RunDeadline, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("unknown RPC outcome must still terminate worker", e)
	}
	if e = s.finishCall(ctx, run, jobs.UUID(), RunSucceeded, RunNoReason, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong invocation lease accepted", e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET grant_revision=grant_revision+1 WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.finishCall(ctx, run, token, RunSucceeded, RunNoReason, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("revocation fence bypassed", e)
	}
}
