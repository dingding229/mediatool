package jobs

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestTaskBrowseAndDetail(t *testing.T) {
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
	repo := Repository{DB: db}
	j, err := repo.Create(ctx, "system.check", json.RawMessage(`{"owned":"browse-fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, j.ID)
	defer db.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, j.ID)
	page, err := repo.Browse(ctx, Filter{Query: j.ID, State: "pending", Kind: "system.check", Queue: "default", Limit: 25})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	if page.Items[0].ID != j.ID || page.Items[0].Queue != "default" || page.Items[0].CreatedAt.IsZero() {
		t.Fatal(page)
	}
	detail, err := repo.Detail(ctx, j.ID)
	if err != nil || string(detail.Payload) != `{"owned": "browse-fixture"}` || detail.LeaseUntil != nil || detail.AvailableAt.IsZero() {
		t.Fatal(detail, err)
	}
	page, err = repo.Browse(ctx, Filter{Query: j.ID, Limit: 25, Offset: 25})
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatal(page, err)
	}
	page, err = repo.Browse(ctx, Filter{Query: "%' OR true --", Limit: 25})
	if err != nil || page.Total != 0 {
		t.Fatal(page, err)
	}
	_, err = repo.Browse(ctx, Filter{Limit: 101})
	if err == nil {
		t.Fatal("unbounded page accepted")
	}
	db.Exec(ctx, `UPDATE jobs SET last_error='owned failure' WHERE id=$1`, j.ID)
	page, err = repo.Browse(ctx, Filter{Query: j.ID, Limit: 25})
	if err != nil || page.Items[0].LastError != "owned failure" {
		t.Fatal(page, err)
	}
}
