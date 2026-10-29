package maintenance

import (
	"context"
	"testing"
)

func TestScheduledWorkerHeartbeatRetention(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	if _, err := e.DB.Exec(ctx, `TRUNCATE worker_heartbeats`); err != nil {
		t.Fatal(err)
	}
	cleanup := func() { _, _ = e.DB.Exec(ctx, `DELETE FROM worker_heartbeats WHERE owner LIKE 'i22-retention-%'`) }
	cleanup()
	t.Cleanup(cleanup)
	if _, err := e.DB.Exec(ctx, `INSERT INTO worker_heartbeats(owner,queue,last_seen_at) VALUES
 ('i22-retention-expired','default',now()-interval '25 hours'),
 ('i22-retention-offline','default',now()-interval '1 hour'),
 ('i22-retention-online','default',now())`); err != nil {
		t.Fatal(err)
	}
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	for owner, want := range map[string]bool{"expired": false, "offline": true, "online": true} {
		var exists bool
		if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM worker_heartbeats WHERE owner=$1)`, "i22-retention-"+owner).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != want {
			t.Errorf("%s exists=%v, want %v", owner, exists, want)
		}
	}
}

func TestScheduledWorkerHeartbeatRetentionIsBoundedAndOnlyRunsWhenDue(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	if _, err := e.DB.Exec(ctx, `TRUNCATE worker_heartbeats`); err != nil {
		t.Fatal(err)
	}
	cleanup := func() { _, _ = e.DB.Exec(ctx, `DELETE FROM worker_heartbeats WHERE owner LIKE 'i22-bounded-%'`) }
	cleanup()
	t.Cleanup(cleanup)
	if _, err := e.DB.Exec(ctx, `INSERT INTO worker_heartbeats(owner,queue,last_seen_at)
 SELECT 'i22-bounded-'||n,'default',now()-interval '25 hours' FROM generate_series(1,1001)n`); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM worker_heartbeats WHERE owner LIKE 'i22-bounded-%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 {
		t.Fatalf("first minute retained %d, want 1 (limit 1000)", n)
	}
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 {
		t.Fatalf("not-due schedule deleted remaining row: %d", n)
	}
	if _, err := e.DB.Exec(ctx, `UPDATE ops_policy SET next_sample_at=now()-interval '1 minute' WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 0 {
		t.Fatalf("next minute retained %d, want 0", n)
	}
}
