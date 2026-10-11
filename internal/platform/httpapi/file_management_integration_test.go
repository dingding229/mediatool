package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgresFileAndCatalogRoutesAndSecretRevealRequireSession(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
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
		t.Fatal("isolated DB required")
	}
	cleanup := `TRUNCATE integrations,subscriptions,download_tasks,storage_connections,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	record, err := repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "Owned secret reveal", Enabled: false, Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "PRIVATE-REVEAL-TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	logs := &bytes.Buffer{}
	h := New(cfg(), Dependencies{DB: db, Integrations: manager, AuthSessions: testSessionReader{}, Logger: slog.New(slog.NewJSONHandler(logs, nil))})
	for _, target := range []string{"/api/v1/files", "/api/v1/files/roots", "/api/v1/files/operations", "/api/v1/media/library", "/api/v1/media/library/emby/11/image"} {
		if w := invoke(h, "GET", target, "", ""); w.Code != 401 {
			t.Fatal("management route accessible without session", target, w.Code)
		}
	}
	body, _ := json.Marshal(map[string]any{"revision": record.Revision, "fields": []string{"token"}, "confirm": true})
	target := "/api/v1/integrations/" + record.ID + "/secrets/reveal"
	if w := invoke(h, "POST", target, string(body), ""); w.Code != 401 {
		t.Fatal("anonymous secret reveal")
	}
	w := invoke(h, "POST", target, string(body), cfg().SessionKey)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "PRIVATE-REVEAL-TOKEN") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("explicit reveal denied or cacheable", w.Code)
	}
	if strings.Contains(logs.String(), "PRIVATE-REVEAL-TOKEN") {
		t.Fatal("secret reveal content logged")
	}
	if w = invoke(h, "GET", "/api/v1/integrations", "", cfg().SessionKey); w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE-REVEAL-TOKEN") {
		t.Fatal("ordinary connection list leaks value")
	}
}
