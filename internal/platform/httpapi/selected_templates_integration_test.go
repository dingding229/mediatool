package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgresSelectedTemplateLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL")
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
	// Restore selections on completion so other integration tests are independent.
	repo := sites.Repository{DB: db}
	before, err := repo.LoadedGoTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='ubits'`)
	db.Exec(ctx, `DELETE FROM site_template_loads`)
	defer func() {
		db.Exec(ctx, `DELETE FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'='ubits'`)
		db.Exec(ctx, `DELETE FROM site_template_loads`)
		for id := range before {
			db.Exec(ctx, `INSERT INTO site_template_loads(template_id) VALUES($1)`, id)
		}
	}()
	v, _ := secrets.New(strings.Repeat("7", 64))
	connections := integrations.Repository{DB: db, Vault: v}
	manager := integrations.NewManager(connections, config.Config{})
	defer manager.Close()
	a := API{Deps: Dependencies{DB: db, Integrations: manager}}
	catalog := func() []map[string]any {
		w := httptest.NewRecorder()
		a.ptTemplates(w, httptest.NewRequest("GET", "/pt/templates", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out struct {
			Data struct{ Templates []map[string]any }
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Data.Templates
	}
	if len(catalog()) != 6 {
		t.Fatal("catalog")
	}
	for _, x := range catalog() {
		if x["loaded"] == true {
			t.Fatal("loaded without selection")
		}
	}
	load := func(raw string) int {
		w := httptest.NewRecorder()
		a.loadPTTemplates(w, httptest.NewRequest("POST", "/pt/templates/load", strings.NewReader(raw)))
		return w.Code
	}
	for _, raw := range []string{`{"ids":[]}`, `{"ids":["hdsky","invalid"]}`} {
		if load(raw) != 400 {
			t.Fatal("invalid selection accepted")
		}
	}
	loaded, _ := repo.LoadedGoTemplates(ctx)
	if len(loaded) != 0 {
		t.Fatal("partial selection committed")
	}
	w := httptest.NewRecorder()
	a.savePTAccount(w, httptest.NewRequest("POST", "/pt/accounts", strings.NewReader(`{"definitionId":"hdsky","values":{"cookie":"owned","userAgent":"Owned-UA"}}`)))
	if w.Code != 400 {
		t.Fatal("unloaded template accepted", w.Code)
	}
	w = httptest.NewRecorder()
	a.saveIntegration(w, httptest.NewRequest("POST", "/integrations", strings.NewReader(`{"kind":"ptsite","name":"HDSKY","enabled":true,"settings":{"definitionId":"hdsky"}}`)))
	if w.Code != 400 {
		t.Fatal("generic bypass", w.Code)
	}
	if load(`{"ids":["hdsky","ssd"]}`) != 200 {
		t.Fatal("load")
	}
	for range 2 {
		if err = repo.RegisterGoTemplates(ctx); err != nil {
			t.Fatal(err)
		}
	}
	loaded, _ = repo.LoadedGoTemplates(ctx)
	if len(loaded) != 2 || !loaded["hdsky"] || !loaded["ssd"] {
		t.Fatal("startup loaded entire catalog", loaded)
	}
	var timestamp1, timestamp2 string
	db.QueryRow(ctx, `SELECT loaded_at::text FROM site_template_loads WHERE template_id='hdsky'`).Scan(&timestamp1)
	if load(`{"ids":["hdsky"]}`) != 200 {
		t.Fatal("reload")
	}
	db.QueryRow(ctx, `SELECT loaded_at::text FROM site_template_loads WHERE template_id='hdsky'`).Scan(&timestamp2)
	if timestamp1 != timestamp2 {
		t.Fatal("not idempotent")
	}
	// Existing disabled accounts must remain editable even without an explicit selection.
	_, err = connections.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "UBITS", Enabled: false, Settings: map[string]any{"definitionId": "ubits", "endpoint": "https://ubits.club"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.RegisterGoTemplates(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = repo.LoadedGoTemplates(ctx)
	if !loaded["ubits"] || len(loaded) != 3 {
		t.Fatal("lost existing account template", loaded)
	}
}
