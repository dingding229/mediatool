package v2

import (
	"context"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/outbox"
	"testing"
)

func execRetention(t *testing.T, s Store, q string, args ...any) {
	t.Helper()
	if _, e := s.DB.Exec(context.Background(), q, args...); e != nil {
		t.Fatal(e)
	}
}
func countRetention(t *testing.T, s Store, q string) int {
	t.Helper()
	var n int
	if e := s.DB.QueryRow(context.Background(), q).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestJournalRetentionLongRunKeepsDedupeAndLateCommits(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	// 10,000 events through 50 GC epochs; each source receipt survives payload GC.
	for cycle := range 50 {
		execRetention(t, s, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload,dispatched_at)SELECT md5($1||':'||n)::uuid,'files.succeeded',md5($1||':'||n)::uuid,'{"state":"done"}',now() FROM generate_series(1,200)n`, jobs.UUID())
		if e := s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		execRetention(t, s, `UPDATE plugin_v2_event_journal SET ingested_at=now()-interval '8 days'`)
		if e := s.CollectJournal(ctx); e != nil {
			t.Fatal(e)
		}
		if n := countRetention(t, s, `SELECT count(*) FROM plugin_v2_event_journal`); n != 0 {
			t.Fatal(cycle, n)
		}
	}
	if n := countRetention(t, s, `SELECT count(*) FROM outbox_events WHERE plugin_compacted AND payload='{}'`); n != 10000 {
		t.Fatal(n)
	}
	// The business idempotency identity remains even after its payload is gone.
	var fact string
	if e := s.DB.QueryRow(ctx, `SELECT aggregate_id::text FROM outbox_events LIMIT 1`).Scan(&fact); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	if e = outbox.EmitTx(ctx, tx, "files.succeeded", fact, map[string]string{"changed": "must not replay"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if n := countRetention(t, s, `SELECT count(*) FROM plugin_v2_event_journal`); n != 0 {
		t.Fatal("re-ingested", n)
	}
	late, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, late)
	if _, e = late.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload,created_at,dispatched_at) VALUES($1,'files.succeeded',$1,'{}',now()-interval '30 days',now())`, jobs.UUID()); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = late.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.CollectJournal(ctx); e != nil {
		t.Fatal(e)
	}
	if n := countRetention(t, s, `SELECT count(*) FROM plugin_v2_event_journal`); n != 1 {
		t.Fatal("late commit lost", n)
	}
}
func TestJournalFullDoesNotStarveScheduleAndRecovers(t *testing.T) {
	s, l, _ := snapshotFixture(t)
	ctx := context.Background()
	execRetention(t, s, `INSERT INTO plugin_v2_event_journal(event_id,event_type,payload,ingested_at) SELECT md5(n::text)::uuid,'library.organized','{}',now()-interval '8 days' FROM generate_series(1,100000)n`)
	execRetention(t, s, `UPDATE plugin_v2_schedules SET next_at=now()-interval '1 minute'`)
	// A slow disabled subscription pins acknowledged history, never auto-advance.
	execRetention(t, s, `INSERT INTO plugin_v2_snapshots(id,instance_id,generation,grant_revision,fingerprint,descriptor_hash,descriptors) SELECT md5('pin')::uuid,instance_id,md5('pin-generation')::uuid,grant_revision,fingerprint,descriptor_hash,descriptors FROM plugin_v2_snapshots LIMIT 1`)
	execRetention(t, s, `UPDATE plugin_v2_subscriptions SET snapshot_id=md5('pin')::uuid`)
	if e := s.MaintainExtensions(ctx); e == nil {
		t.Fatal("full journal did not report pressure")
	}
	if n := countRetention(t, s, `SELECT count(*) FROM jobs WHERE kind='plugin.v2.service'`); n == 0 {
		t.Fatal("journal starved Schedule")
	}
	capacity, e := s.JournalCapacity(ctx)
	if e != nil || capacity.State != "blocked" {
		t.Fatal(capacity, e)
	}
	execRetention(t, s, `UPDATE plugin_v2_subscriptions SET cursor=(SELECT max(cursor)FROM plugin_v2_event_journal)`)
	if e = s.MaintainExtensions(ctx); e != nil {
		t.Fatal(e)
	}
	capacity, e = s.JournalCapacity(ctx)
	if e != nil || capacity.State != "warning" {
		t.Fatal("recoverable warning", capacity, e)
	}
	// Recovery is bounded, 1k per pass, and returns to ready automatically.
	for range 100 {
		if e = s.CollectJournal(ctx); e != nil {
			t.Fatal(e)
		}
	}
	capacity, e = s.JournalCapacity(ctx)
	if e != nil || capacity.State != "ready" || capacity.Stored != 0 {
		t.Fatal(capacity, e)
	}
	var expired *ReplayExpired
	if _, e = s.Replay(ctx, "admin", l.ID, 0, 100, false); !errors.As(e, &expired) {
		t.Fatal("expired cursor not explicit", e)
	}
}
func TestJournalGCRespectsUnackedUnknownAndNotificationDispatch(t *testing.T) {
	s, l, _ := snapshotFixture(t)
	ctx := context.Background()
	execRetention(t, s, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES(md5('pin')::uuid,'files.succeeded',md5('pin')::uuid,'{}')`)
	if e := s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	execRetention(t, s, `UPDATE plugin_v2_event_journal SET ingested_at=now()-interval '8 days'`)
	execRetention(t, s, `UPDATE plugin_v2_runs SET state='unknown'`)
	if e := s.CollectJournal(ctx); e != nil {
		t.Fatal(e)
	}
	if n := countRetention(t, s, `SELECT count(*) FROM plugin_v2_event_journal`); n != 1 {
		t.Fatal("unknown lost")
	}
	execRetention(t, s, `UPDATE plugin_v2_runs SET state='succeeded',finished_at=now()-interval '8 days'`)
	if e := s.CollectJournal(ctx); e != nil {
		t.Fatal(e)
	}
	if n := countRetention(t, s, `SELECT count(*) FROM outbox_events WHERE plugin_compacted`); n != 0 {
		t.Fatal("pending notification lost payload")
	}
	execRetention(t, s, `UPDATE outbox_events SET dispatched_at=now()`)
	if e := s.CollectJournal(ctx); e != nil {
		t.Fatal(e)
	}
	if n := countRetention(t, s, `SELECT count(*) FROM outbox_events WHERE plugin_compacted`); n != 1 {
		t.Fatal(n)
	}
	// Late replay after pruning is rejected even with confirmUnknown=true.
	var expired *ReplayExpired
	if _, e := s.Replay(ctx, "admin", l.ID, 0, 100, true); !errors.As(e, &expired) {
		t.Fatal(e)
	}
}
