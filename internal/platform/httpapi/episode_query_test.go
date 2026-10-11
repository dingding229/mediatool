package httpapi

import (
	"context"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestEpisodePreviewRejectsAmbiguousQueriesBeforeProviderCalls(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
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
		t.Fatal("isolated media_tool_test required")
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg())
	defer m.Close()
	api := &API{Deps: Dependencies{DB: db, Integrations: m}}
	for _, query := range []string{"tmdbId=1&season=1&episodes=1&episodes=2", "tmdbId=1&season=1&episodes=1,1", "tmdbId=1&season=-1&episodes=1", "tmdbId=1&season=1&episodes=1,", "tmdbId=1&season=1&episodes=1&cookie=private", "tmdbId=1&season=1&Episodes=1"} {
		w := httptest.NewRecorder()
		api.metadataEpisodes(w, httptest.NewRequest("GET", "http://owned/metadata/episodes?"+query, nil))
		if w.Code != 400 || strings.Contains(w.Body.String(), "private") {
			t.Fatal("query not rejected", query, w.Code)
		}
	}
	handler := New(cfg(), Dependencies{DB: db, Integrations: m})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://owned/api/v1/metadata/episodes?tmdbId=1&season=1&episodes=1", nil))
	if w.Code != 401 {
		t.Fatal("anonymous episode lookup accepted", w.Code)
	}
}
