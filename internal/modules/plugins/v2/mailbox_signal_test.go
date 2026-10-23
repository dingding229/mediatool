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

// Exercise the actual pump with its predecessor still registered. This models
// overlapping supervisor lifetimes; hint ownership must never veto a DB lease.
func TestMailboxRestartReplacesOldLaneBeforeDeferredCleanup(t *testing.T) {
	s, l := activeFixture(t)
	key := mailboxKey{s.DB, l.ID}
	old := &mailboxLane{wake: make(chan struct{}, 1), changed: make(chan struct{})}
	mailboxLanes.Store(key, old)
	defer mailboxLanes.Delete(key)
	oldObserver := old.observed()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var pumpErr error
	go func() {
		// There are no queued runs, so no peer is needed. The real claim path and
		// pump registration/teardown still execute against the isolated database.
		pumpErr = s.pump(ctx, l, nil, &hostGate{})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			if pumpErr != nil && !errors.Is(pumpErr, context.Canceled) {
				t.Error(pumpErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("replacement pump cleanup timeout")
		}
	})
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	var current *mailboxLane
	for current == nil || current == old {
		current = s.mailbox(l.ID)
		if current != nil && current != old {
			break
		}
		select {
		case <-done:
			t.Fatalf("new pump rejected predecessor's advisory lane: %v", pumpErr)
		case <-deadline.C:
			t.Fatal("new pump did not register replacement lane")
		case <-time.After(time.Millisecond):
		}
	}
	// Run the predecessor's deferred operations *after* replacement. It must
	// wake its own observers without deleting or broadcasting on the new lane.
	currentObserver := current.observed()
	old.completed()
	if mailboxLanes.CompareAndDelete(key, old) {
		t.Fatal("old pump deleted new lane")
	}
	if s.mailbox(l.ID) != current {
		t.Fatal("replacement lane lost")
	}
	select {
	case <-oldObserver:
	default:
		t.Fatal("old observers stranded")
	}
	select {
	case <-currentObserver:
		t.Fatal("old cleanup broadcast on replacement lane")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-deadline.C:
		t.Fatal("replacement pump did not stop")
	}
	if s.mailbox(l.ID) != nil {
		t.Fatal("replacement pump leaked its lane")
	}
	select {
	case <-currentObserver:
	default:
		t.Fatal("replacement observers stranded on exit")
	}
}

func TestScriptedPolicyPeerPublishesCommittedCompletion(t *testing.T) {
	s, l := policyFixture(t)
	done := policyReply(s, json.RawMessage(`{"ids":[]}`), false)
	lane := s.mailbox(l.ID)
	if lane == nil {
		t.Fatal("fake peer readiness omitted real pump lane")
	}
	changed := lane.observed()
	snap, d, e := s.Descriptor(t.Context(), l.ID, "chain", "resource.selection")
	if e != nil {
		t.Fatal(e)
	}
	id, e := s.Submit(t.Context(), snap, d, json.RawMessage(`{}`), "fake-signal:"+jobs.UUID(), "", nil, time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("committed fake reply did not notify waiter")
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	var state RunState
	if e = s.DB.QueryRow(t.Context(), `SELECT state FROM plugin_v2_runs WHERE id=$1`, id).Scan(&state); e != nil || state != RunSucceeded {
		t.Fatal("wake before durable reply", state, e)
	}
	if s.mailbox(l.ID) != nil {
		t.Fatal("finished fake peer lane retained")
	}
}
