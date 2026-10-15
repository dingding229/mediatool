package messaging

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestUnknownDeliveryRequiresAcknowledgementAndCrashIsNotBlindlyRetried(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
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
		t.Fatal("isolated database required")
	}
	cleanup := `TRUNCATE notification_messages,notification_deliveries,outbox_events,integrations CASCADE`
	db.Exec(ctx, cleanup)
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	c, err := repo.Save(ctx, "", integrations.Input{Kind: "telegram", Name: "owned target", Enabled: true, Settings: map[string]any{"chatId": "123"}, Secrets: map[string]string{"botToken": "owned-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	dispatcher := Dispatcher{DB: db, Connections: manager}
	event, id := jobs.UUID(), jobs.UUID()
	body, _ := json.Marshal(map[string]string{"title": "owned"})
	db.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.imported',$2,$3)`, event, jobs.UUID(), body)
	db.Exec(ctx, `INSERT INTO notification_messages(id,event_id,title,body)VALUES($1,$2,'owned','owned')`, jobs.UUID(), event)
	db.Exec(ctx, `INSERT INTO notification_deliveries(id,event_id,integration_id,integration_revision,state,started_at)VALUES($1,$2,$3,$4,'sending',now()-interval '5 minutes')`, id, event, c.ID, c.Revision)
	if err = dispatcher.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM notification_deliveries WHERE id=$1`, id).Scan(&state)
	if state != "outcome_unknown" {
		t.Fatal("crashed sender claimed safe retry", state)
	}
	if err = dispatcher.Retry(ctx, id, false); err == nil {
		t.Fatal("unknown send retried without acknowledgement")
	}
	if err = dispatcher.Retry(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `SELECT state FROM notification_deliveries WHERE id=$1`, id).Scan(&state)
	if state != "pending" {
		t.Fatal(state)
	}
}

func TestStaleNotificationAttemptCannotOverwriteRetry(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Skip("isolated database required")
	}
	cleanup := `TRUNCATE notification_messages,notification_deliveries,outbox_events CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, cleanup)
	eventID, deliveryID := jobs.UUID(), jobs.UUID()
	if _, err = db.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.imported',$2,'{}')`, eventID, jobs.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO notification_messages(id,event_id,title,body)VALUES($1,$2,'owned','owned')`, jobs.UUID(), eventID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO notification_deliveries(id,event_id,state,attempt_token)VALUES($1,$2,'pending','new-attempt')`, deliveryID, eventID); err != nil {
		t.Fatal(err)
	}
	d := Dispatcher{DB: db}
	if err = d.finishAttempt(ctx, deliveryID, "old-attempt", "sent", "provider", "", 0); err == nil {
		t.Fatal("stale attempt overwrote a newer pending retry")
	}
	var state, token string
	if err = db.QueryRow(ctx, `SELECT state,attempt_token FROM notification_deliveries WHERE id=$1`, deliveryID).Scan(&state, &token); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || token != "new-attempt" {
		t.Fatalf("stale attempt changed delivery: state=%s token=%s", state, token)
	}
}
