package system

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestOperationsUnavailableIsNotHealthy(t *testing.T) {
	out := Inspect(context.Background(), nil, nil)
	if out.PostgreSQL.State != "unavailable" || out.Redis.State != "unavailable" || out.DatabaseBytes != nil || out.Migration != nil || out.PendingNotifications != nil || len(out.Unavailable) == 0 {
		t.Fatal(out)
	}
	if out.Workers == nil || out.Schedules == nil || out.Queues == nil || out.Process["goroutines"] == 0 {
		t.Fatal("invalid snapshot")
	}
}
func TestOperationsReadsRealIsolatedDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	_, err = db.Exec(ctx, `INSERT INTO worker_heartbeats(owner,queue,last_seen_at)VALUES('ops-owned-online','default',now()),('ops-owned-stale','upload',now()-interval '1 minute')`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM worker_heartbeats WHERE owner LIKE 'ops-owned-%'`)
	out := Inspect(ctx, db, nil)
	if out.PostgreSQL.State != "ready" || out.Migration == nil || *out.Migration < 49 || out.DatabaseBytes == nil || len(out.Unavailable) != 0 {
		t.Fatal(out)
	}
	online, stale := false, false
	for _, w := range out.Workers {
		if w.Owner == "ops-owned-online" {
			online = w.Online
		}
		if w.Owner == "ops-owned-stale" {
			stale = !w.Online
		}
	}
	if !online || !stale {
		t.Fatal(out.Workers)
	}
	if len(out.Schedules) < 1 || out.Schedules[0].Cron == "" {
		t.Fatal(out.Schedules)
	}
}
