package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgres115AuthorizationDraftAndExplicitSecretReveal(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated database only")
	}
	cleanup := `TRUNCATE integrations,resources,site_definitions,search_runs,subscriptions,download_tasks,storage_connections CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	draft, err := repo.Save(ctx, "", integrations.Input{Kind: "open115", Name: "Owned pending authorization", Enabled: true, Settings: map[string]any{"appId": "123456"}})
	if err != nil || draft.Enabled || len(draft.SecretFields) != 0 || draft.Settings["cloudMountPrefix"] != "/115" {
		t.Fatal("tokenless draft cannot save safely", err)
	}
	record, err := repo.Save(ctx, draft.ID, integrations.Input{Kind: "open115", Name: draft.Name, Revision: draft.Revision, Enabled: true, Settings: draft.Settings, Secrets: map[string]string{"accessToken": "PRIVATE-ACCESS", "refreshToken": "PRIVATE-REFRESH"}})
	if err != nil || !record.Enabled {
		t.Fatal(err)
	}
	public, _ := json.Marshal(record)
	if strings.Contains(string(public), "PRIVATE-") {
		t.Fatal("ordinary DTO leaks secrets")
	}
	values, err := repo.Reveal(ctx, record.ID, record.Revision, []string{"accessToken"})
	if err != nil || len(values) != 1 || values["accessToken"] != "PRIVATE-ACCESS" {
		t.Fatal("explicit reveal failed", err)
	}
	for _, fields := range [][]string{{"refreshBlocked"}, {"accessToken", "accessToken"}, {"masterKey"}} {
		if _, err = repo.Reveal(ctx, record.ID, record.Revision, fields); !errors.Is(err, integrations.ErrInvalid) {
			t.Fatal("internal/invalid fields disclosed", err)
		}
	}
	if _, err = repo.Reveal(ctx, record.ID, record.Revision-1, []string{"accessToken"}); !errors.Is(err, integrations.ErrRevision) {
		t.Fatal("stale credential reveal allowed", err)
	}
	var logs string
	if err = db.QueryRow(ctx, `SELECT coalesce(json_agg(details),'[]'::json)::text FROM audit_logs WHERE action='integration.secrets.reveal'`).Scan(&logs); err != nil || strings.Contains(logs, "PRIVATE-") {
		t.Fatal("secret appeared in audit", err)
	}
	// Blank omission preserves the latest server tokens, as the UI excludes unchanged echoes.
	edited, err := repo.Save(ctx, record.ID, integrations.Input{Kind: "open115", Name: record.Name, Revision: record.Revision, Enabled: true, Settings: record.Settings})
	if err != nil || !edited.Enabled {
		t.Fatal(err)
	}
	values, err = repo.Reveal(ctx, edited.ID, edited.Revision, []string{"refreshToken"})
	if err != nil || values["refreshToken"] != "PRIVATE-REFRESH" {
		t.Fatal("unmodified token lost")
	}
}
