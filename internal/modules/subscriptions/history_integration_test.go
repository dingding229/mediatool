package subscriptions

import (
	"context"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestArchivedSubscriptionsPreserveEvaluationAndRejectScheduling(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	id, run := jobs.UUID(), jobs.UUID()
	defer db.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, id)
	_, err = db.Exec(ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled,source,interval_seconds) VALUES($1,'Owned archived subscription','movie','{"id":42,"title":"Owned","mediaType":"movie"}','{}',false,'search',1800)`, id)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `INSERT INTO subscription_runs(id,subscription_id,preview,state,revision) VALUES($1,$2,true,'succeeded',1)`, run, id)
	e := Engine{DB: db}
	if err = e.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	record, err := e.Get(ctx, id)
	if err != nil || record.ArchivedAt == nil || record.Enabled {
		t.Fatal(record, err)
	}
	history, err := e.History(ctx, id)
	if err != nil || len(history) != 1 || history[0].ID != run {
		t.Fatal(history, err)
	}
	if _, err = e.Queue(ctx, id, false); err == nil {
		t.Fatal("archived scheduled")
	}
	archived, err := e.Browse(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range archived {
		found = found || r.ID == id
	}
	if !found {
		t.Fatal("archive hidden")
	}
}
