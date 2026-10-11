package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/platform/database"
	"os"
	"testing"
	"time"
)

// Opt in with TEST_DATABASE_URL pointing to a migrated disposable database.
func TestPostgresJobLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration not executed")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := Repository{DB: db}
	j, err := repo.Create(ctx, "system.check", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, j.ID)
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, j.ID)
	claimed, err := repo.Claim(ctx, "test", time.Minute)
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if claimed.ID != j.ID {
		t.Fatal("integration test database must have an empty queue")
	}
	if err = repo.Heartbeat(ctx, *claimed, time.Minute); err != nil {
		t.Fatal(err)
	}
	old := *claimed
	old.LeaseToken = UUID()
	if err = repo.Finish(ctx, old, nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatal("stale lease accepted", err)
	}
	if err = repo.Finish(ctx, *claimed, nil); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM jobs WHERE id=$1`, j.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatal(state, err)
	}
}

func TestOperatorRequiredJobPausesWithoutAutomaticRetry(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
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
		t.Fatal("isolated PG required")
	}
	repo := Repository{DB: db}
	j, err := repo.Create(ctx, "system.check", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, j.ID)
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, j.ID)
	claimed, err := repo.Claim(ctx, "operator-fixture", time.Minute)
	if err != nil || claimed == nil || claimed.ID != j.ID {
		t.Fatal("isolated queue not empty")
	}
	if err = repo.Finish(ctx, *claimed, errors.Join(ErrOperatorRequired, errors.New("explicit layout migration required"))); err != nil {
		t.Fatal(err)
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM jobs WHERE id=$1`, j.ID).Scan(&state)
	if state != "paused" {
		t.Fatal("operator-required task automatically retries", state)
	}
	next, err := repo.Claim(ctx, "must-not-retry", time.Minute)
	if err != nil || next != nil {
		t.Fatal("paused task claimed without approval")
	}
}
