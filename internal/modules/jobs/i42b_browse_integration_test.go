package jobs

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/database"
	"os"
	"testing"
	"time"
)

func TestI42bUnresolvedAndRoutineBrowse(t *testing.T) {
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
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal(name, err)
	}
	repo := Repository{DB: db}
	at := time.Now().UTC()
	ids := []string{}
	add := func(kind, state, object string, delta int) string {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"accountId": object})
		j, e := repo.Create(ctx, kind, raw)
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, j.ID)
		if _, e = db.Exec(ctx, `UPDATE jobs SET state=$2,updated_at=$3 WHERE id=$1`, j.ID, state, at.Add(time.Duration(delta)*time.Second)); e != nil {
			t.Fatal(e)
		}
		return j.ID
	}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, id)
			db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, id)
		}
	}()
	kind := "site.metrics.refresh"
	object := "i42b-object-" + at.Format(time.RFC3339Nano)
	first := add(kind, "failed", object, 0)
	count := func(id string, unresolved, hide bool) int {
		t.Helper()
		p, e := repo.Browse(ctx, Filter{Query: id, Limit: 25, UnresolvedOnly: unresolved, HideRoutineSuccess: hide})
		if e != nil {
			t.Fatal(e)
		}
		return p.Total
	}
	if count(first, true, false) != 1 {
		t.Fatal("failure missing")
	}
	add(kind, "succeeded", object+"-other", 1)
	if count(first, true, false) != 1 {
		t.Fatal("another object recovered failure")
	}
	add("system.check", "succeeded", object, 2)
	if count(first, true, false) != 1 {
		t.Fatal("another kind recovered failure")
	}
	success := add(kind, "succeeded", object, 3)
	if count(first, true, false) != 0 || count(first, false, false) != 1 {
		t.Fatal("recovered failure must only disappear from attention")
	}
	again := add(kind, "failed", object, 4)
	if count(again, true, false) != 1 {
		t.Fatal("subsequent failure missing")
	}
	if count(success, false, true) != 0 || count(again, false, true) != 1 {
		t.Fatal("only routine success may be hidden")
	}
	nonroutine := add("system.check", "succeeded", object, 5)
	if count(nonroutine, false, true) != 1 {
		t.Fatal("non-routine success hidden")
	}
}
