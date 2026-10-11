package catalog

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPostgresEmbyHomeIsUserScopedAndKeysStayServerSide(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
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
		t.Fatal("test DB required")
	}
	db.Exec(ctx, "TRUNCATE integrations CASCADE")
	defer db.Exec(ctx, "TRUNCATE integrations CASCADE")
	token := "owned-emby-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != token {
			t.Error("missing server token")
		}
		switch r.URL.Path {
		case "/Users":
			json.NewEncoder(w).Encode([]any{map[string]any{"Id": "u1", "Name": "Owned"}})
		case "/Users/u1/Views":
			json.NewEncoder(w).Encode(map[string]any{"Items": []any{map[string]any{"Id": "movies", "Name": "Movies", "Type": "CollectionFolder", "ImageTags": map[string]string{"Primary": "tag"}}}})
		case "/Users/u1/Items/Latest":
			json.NewEncoder(w).Encode([]any{map[string]any{"Id": "movie1", "Name": "Owned movie", "Type": "Movie"}})
		case "/Sessions":
			json.NewEncoder(w).Encode([]any{map[string]any{"UserId": "other", "NowPlayingItem": map[string]any{"Id": "private", "Name": "Not your stream"}}, map[string]any{"UserId": "u1", "NowPlayingItem": map[string]any{"Id": "current", "Name": "Owned playing", "Type": "Movie"}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"Items": []any{}})
		}
	}))
	defer server.Close()
	v, _ := secrets.New(strings.Repeat("b", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	_, err = repo.Save(ctx, "", integrations.Input{Kind: "emby", Name: "Owned Emby", Enabled: true, Settings: map[string]any{"endpoint": server.URL}, Secrets: map[string]string{"apiKey": token}})
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	e := Engine{DB: db, Connections: manager}
	home, err := e.Home(ctx, "u1")
	if err != nil || len(home.Libraries) != 1 || len(home.Latest) != 1 || len(home.Playing) != 1 || home.Playing[0].ID != "current" {
		t.Fatal(home, err)
	}
	raw, _ := json.Marshal(home)
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), "Not your stream") {
		t.Fatal("scope/key leak")
	}
	if _, err = e.Home(ctx, "invalid-user"); err == nil {
		t.Fatal("invalid user accepted")
	}
}
