package playeraccess

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestEventExactUserFilterAndAddressEcho(t *testing.T) {
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
	db.QueryRow(ctx, "select current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	db.Exec(ctx, "TRUNCATE player_access_events")
	defer db.Exec(ctx, "TRUNCATE player_access_events")
	r := Repository{DB: db}
	for _, u := range []string{"user-a", "user-b", ""} {
		id, err := r.RecordEvent(ctx, Request{UserAgent: "Owned", Entry: "emby", ClientIP: "8.8.8.8", ConnectionIP: "10.0.0.2", IPSource: "trusted_proxy", IPPublic: true}, Decision{Allowed: true, PolicyRevision: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err = r.FinishEvent(ctx, id, u, "same-name", u != "", 302); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []string{"user-a", "user-b", "unknown"} {
		out, err := r.FilteredEvents(ctx, "", "", u, 1, 25)
		if err != nil || out.Total != 1 || len(out.Items) != 1 {
			t.Fatal(out, err)
		}
		e := out.Items[0]
		if e.ClientIP != "8.8.8.8" || e.ConnectionIP != "" || !e.IPPublic || e.IPSource != "trusted_proxy" {
			t.Fatal(e)
		}
	}
}
