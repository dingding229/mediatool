package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgresEmbyCatalogIsReadonlyAndNeverReturnsProviderSecrets(t *testing.T) {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "PRIVATE-EMBY-KEY" || r.Method != "GET" {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/Items" {
			if r.URL.Query().Get("Limit") != "24" || r.URL.Query().Get("StartIndex") != "0" || r.URL.Query().Get("Fields") != "Overview" {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Items":[{"Id":"11","Name":"Owned Movie","Type":"Movie","ProductionYear":2026,"Overview":"Owned synopsis","ImageTags":{"Primary":"opaque"},"Path":"PRIVATE-SIGNED-URL","MediaSources":[{"Path":"PRIVATE-PLAYBACK-KEY"}]}],"TotalRecordCount":1}`))
			return
		}
		if r.URL.Path == "/Items/11/Images/Primary" {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("owned-fixture-image"))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "emby", Name: "Owned Emby", Enabled: true, Settings: map[string]any{"endpoint": server.URL}, Secrets: map[string]string{"apiKey": "PRIVATE-EMBY-KEY"}}); err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	e := Engine{DB: db, Connections: manager}
	out, err := e.List(ctx, "emby", "", "movie", 1, 24)
	if err != nil || out.Total != 1 || len(out.Items) != 1 || out.Items[0].Title != "Owned Movie" || out.Items[0].Poster != "/api/v1/media/library/emby/11/image" {
		t.Fatal(out, err)
	}
	public, _ := json.Marshal(out)
	if strings.Contains(string(public), "PRIVATE-") {
		t.Fatal("private Emby source or key escaped catalog")
	}
	if _, mime, err := e.Image(ctx, "11"); err != nil || mime != "image/png" {
		t.Fatal(err)
	}
	if _, _, err = e.Image(ctx, "../11"); err == nil {
		t.Fatal("poster URL traversal")
	}
	if _, err = e.List(ctx, "managed", "", "", 1, 24); err != nil {
		t.Fatal("empty managed library", err)
	}
}
