package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func TestEventReplayHorizonUsesOriginalFactAndQueuedExecution(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	_, d, e := s.Descriptor(ctx, l.ID, "event", "library.organized")
	if e != nil {
		t.Fatal(e)
	}
	d.ReplayWindowSeconds = 7 * 86400
	// A fresh immutable snapshot at the same generation is not publishable; this
	// fixture updates metadata directly only to exercise host event queue guards.
	for i := range snap.Metadata.Descriptors {
		if snap.Metadata.Descriptors[i].Kind == "event" {
			snap.Metadata.Descriptors[i] = d
		}
	}
	raw, _ := json.Marshal(snap.Metadata)
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_snapshots SET descriptors=$2 WHERE id=$1`, snap.ID, raw); e != nil {
		t.Fatal(e)
	}
	for _, age := range []time.Duration{time.Hour, 8 * 24 * time.Hour} {
		id := jobs.UUID()
		occurred := time.Now().Add(-age).UTC()
		var cursor int64
		if e = s.DB.QueryRow(ctx, `INSERT INTO plugin_v2_event_journal(event_id,event_type,payload,created_at)VALUES($1,'library.organized','{}',$2)RETURNING cursor`, id, occurred).Scan(&cursor); e != nil {
			t.Fatal(e)
		}
		run, err := s.eventRun(ctx, snap, d, cursor, id, json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if age > 7*24*time.Hour {
			if run != "" {
				t.Fatal("expired fact queued")
			}
			continue
		}
		if run == "" {
			t.Fatal("fresh fact lost")
		}
		var cipher []byte
		if e = s.DB.QueryRow(ctx, `SELECT args_ciphertext FROM plugin_v2_runs WHERE id=$1`, run).Scan(&cipher); e != nil {
			t.Fatal(e)
		}
		opened, e := s.open("call-args", run, cipher)
		if e != nil {
			t.Fatal(e)
		}
		var env events.Envelope
		if e = json.Unmarshal(opened, &env); e != nil || env.OccurredAt.Unix() != occurred.Unix() {
			t.Fatal(e, env)
		}
		if ok, e := s.queuedEventFresh(ctx, run); e != nil || !ok {
			t.Fatal(ok, e)
		}
		if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_event_journal SET created_at=now()-interval '8 days' WHERE cursor=$1`, cursor); e != nil {
			t.Fatal(e)
		}
		if ok, e := s.queuedEventFresh(ctx, run); e != nil || ok {
			t.Fatal("stale pending still runnable", ok, e)
		}
	}
	n, e := s.Replay(ctx, "admin", l.ID, 0, 200, true)
	if e != nil || n != 0 {
		t.Fatal("manual replay bypassed horizon", n, e)
	}
	now := time.Now()
	if !eventFresh(contracts.Descriptor{}, now.Add(-100*24*time.Hour), now) {
		t.Fatal("legacy replay changed")
	}
}
