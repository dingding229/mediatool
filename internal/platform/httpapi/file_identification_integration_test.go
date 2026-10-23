package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPostgresFileIdentificationAndIngestionClassificationGolden(t *testing.T) {
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
	if db.QueryRow(ctx, "SELECT current_database()").Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated test database required")
	}
	cleanup := `TRUNCATE integrations,storage_connections,subscriptions,download_tasks,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, cleanup)
	oldPolicy, err := (classification.Repository{DB: db}).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldRaw, _ := json.Marshal(oldPolicy.Value)
	defer db.Exec(ctx, `UPDATE directory_classification SET value=$1,revision=$2 WHERE singleton`, oldRaw, oldPolicy.Revision)
	policy := classification.Defaults()
	raw, _ := json.Marshal(policy)
	if _, err = db.Exec(ctx, `UPDATE directory_classification SET value=$1 WHERE singleton`, raw); err != nil {
		t.Fatal(err)
	}
	prefRepo := settings.Repository{DB: db}
	oldPrefs, err := prefRepo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prefsRaw, _ := json.Marshal(oldPrefs.Value)
	defer db.Exec(ctx, `UPDATE application_settings SET value=$1,revision=$2 WHERE singleton`, prefsRaw, oldPrefs.Revision)
	prefs := oldPrefs.Value
	prefs.MovieTemplate = "{title} ({year})/{title} ({year}) - {videoFormat}"
	prefs.SeriesTemplate = "{title} ({year})/Season {season}/{title} - {season_episode}"
	if _, err = prefRepo.Save(ctx, prefs, oldPrefs.Revision); err != nil {
		t.Fatal(err)
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "F6 fixture TMDB", Enabled: true, Settings: map[string]any{"language": "en-US"}, Secrets: map[string]string{"token": "fixture-not-a-real-token"}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, err := repo.Save(ctx, "", integrations.Input{Kind: "local", Name: "F6 fixture files", Enabled: true, Settings: map[string]any{"rootPath": dir}})
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, cfg())
	defer manager.Close()
	var cases []struct {
		Source          string
		Media           metadata.Item
		Season, Episode int
		Path, Category  string
	}
	raw, err = os.ReadFile("../../modules/filemanager/testdata/organize-layout.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("golden fixture")
	}
	offline := false
	manager.OutboundHTTP = &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" {
			t.Fatalf("unexpected provider %s", r.URL.Host)
		}
		payload := any(map[string]any{"results": []any{}})
		status := 200
		if offline {
			status = 502
		} else if strings.Contains(r.URL.Path, "/search/") {
			matches := []any{}
			for _, c := range cases {
				if strings.Contains(r.URL.Query().Get("query"), c.Media.Title) {
					kind := "movie"
					if c.Media.MediaType == "series" {
						kind = "tv"
					}
					matches = append(matches, map[string]any{"id": c.Media.ID, "title": c.Media.Title, "name": c.Media.Title, "media_type": kind, "release_date": "2026-01-01", "first_air_date": "2026-01-01", "poster_path": c.Media.PosterPath})
				}
			}
			if r.URL.Query().Get("query") == "Ambiguous" {
				for _, id := range []int{401, 402} {
					matches = append(matches, map[string]any{"id": id, "title": "Ambiguous", "media_type": "movie", "release_date": "2026-01-01"})
				}
			}
			payload = map[string]any{"results": matches}
		} else {
			for _, c := range cases {
				if strings.HasSuffix(r.URL.Path, "/"+strconv.Itoa(c.Media.ID)) {
					genres := []any{}
					for _, id := range c.Media.GenreIDs {
						genres = append(genres, map[string]any{"id": id, "name": "fixture genre"})
					}
					payload = map[string]any{"id": c.Media.ID, "title": c.Media.Title, "name": c.Media.Title, "original_title": c.Media.OriginalTitle, "original_name": c.Media.OriginalTitle, "release_date": "2026-01-01", "first_air_date": "2026-01-01", "origin_country": c.Media.Countries, "original_language": c.Media.OriginalLanguage, "genres": genres, "poster_path": c.Media.PosterPath}
				}
			}
		}
		body, _ := json.Marshal(payload)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}
	h := New(cfg(), Dependencies{DB: db, Integrations: manager, AuthSessions: testSessionReader{}})
	if w := invoke(h, "POST", "/api/v1/files/identify", `{"name":"Owned.mkv"}`, ""); w.Code != 401 {
		t.Fatal("anonymous identification", w.Code)
	}
	identify := func(input any) map[string]json.RawMessage {
		body, _ := json.Marshal(input)
		w := invoke(h, "POST", "/api/v1/files/identify", string(body), cfg().SessionKey)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out struct{ Data map[string]json.RawMessage }
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("identity response")
		}
		return out.Data
	}
	for _, c := range cases {
		t.Run(c.Media.MediaType, func(t *testing.T) {
			out := identify(map[string]any{"name": c.Source})
			var selected metadata.Item
			var match classification.Match
			if json.Unmarshal(out["selected"], &selected) != nil || selected.ID != c.Media.ID {
				t.Fatalf("unique exact identity not selected: %s", out["selected"])
			}
			if json.Unmarshal(out["classification"], &match) != nil || match.Directory != c.Category {
				t.Fatalf("details classification=%s", out["classification"])
			}
			ingestion, expected, err := classification.Render(policy, selected, c.Source, prefs.MovieTemplate, prefs.SeriesTemplate, ".mkv", c.Season, []int{c.Episode})
			if err != nil || ingestion != c.Path || expected.Directory != match.Directory {
				t.Fatal(ingestion, err)
			}
			if err = os.WriteFile(filepath.Join(dir, c.Source), []byte("owned synthetic fixture only"), 0600); err != nil {
				t.Fatal(err)
			}
			input := filemanager.Input{ConnectionID: root.ID, Source: c.Source, Operation: "organize", MediaType: selected.MediaType, TMDBID: selected.ID, Category: match.Directory, Season: c.Season, Episode: c.Episode}
			body, _ := json.Marshal(input)
			w := invoke(h, "POST", "/api/v1/files/preview", string(body), cfg().SessionKey)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var plan struct{ Data filemanager.Plan }
			if json.Unmarshal(w.Body.Bytes(), &plan) != nil || len(plan.Data.Items) != 1 || plan.Data.Items[0].Target != ingestion || plan.Data.Media.ID != selected.ID || plan.Data.Category != c.Category {
				t.Fatalf("preview not equal to ingestion golden: %s", w.Body.String())
			}
			if _, err = os.Stat(filepath.Join(dir, c.Source)); err != nil {
				t.Fatal("preview moved source", err)
			}
			if _, err = os.Stat(filepath.Join(dir, ingestion)); !os.IsNotExist(err) {
				t.Fatal("preview wrote target", err)
			}
			engine := filemanager.Engine{DB: db, Connections: manager}
			executed, err := engine.Execute(ctx, plan.Data.ID, plan.Data.Digest, true)
			if err != nil || executed.State != "succeeded" {
				t.Fatal("test fixture organization failed", executed.State, err)
			}
			if _, err = os.Stat(filepath.Join(dir, ingestion)); err != nil {
				t.Fatal("organized fixture missing", err)
			}
			if _, err = os.Stat(filepath.Join(dir, c.Source)); !os.IsNotExist(err) {
				t.Fatal("test source not moved", err)
			}
			chosen := identify(map[string]any{"name": "unidentified.bin", "tmdbId": c.Media.ID, "mediaType": c.Media.MediaType})
			if string(chosen["classification"]) != string(out["classification"]) {
				t.Fatal("manual identity diverged")
			}
		})
	}
	var selected *metadata.Item
	out := identify(map[string]any{"name": "Ambiguous.2026.mkv"})
	json.Unmarshal(out["selected"], &selected)
	if selected != nil || string(out["warning"]) == `""` {
		t.Fatal("ambiguous identity auto-selected")
	}
	out = identify(map[string]any{"name": "Unknown.mkv", "query": "Owned Film", "mediaType": "movie"})
	var matches []metadata.Item
	json.Unmarshal(out["matches"], &matches)
	if len(matches) != 1 || string(out["selected"]) != "null" {
		t.Fatal("manual search must let user choose")
	}
	offline = true
	out = identify(map[string]any{"name": "Owned.Film.2026.mkv"})
	if !strings.Contains(string(out["warning"]), "未识别") || len(out["categories"]) == 0 {
		t.Fatal("offline recognition blocked manual categories")
	}
	for _, body := range []string{`{"name":""}`, `{"name":"x","tmdbId":-1}`, `{"name":"x","tmdbId":1,"mediaType":"auto"}`} {
		if w := invoke(h, "POST", "/api/v1/files/identify", body, cfg().SessionKey); w.Code != 400 {
			t.Fatal("invalid identity accepted", w.Code)
		}
	}
}
