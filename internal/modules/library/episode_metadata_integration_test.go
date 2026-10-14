package library

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestConfirmedSparseEpisodesHaveExactFilenameAssetMetadataAndShowIsolation(t *testing.T) {
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
	db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	defer db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	root := t.TempDir()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{PublicURL: "https://owned.fixture", STRMOutputDir: root, STRMAllowedRoots: []string{"/115/Owned"}})
	defer m.Close()
	cloud, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "owned episode fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115/Owned"}, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	tmdb, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "owned metadata fixture", Enabled: true, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE id IN($1,$2)`, cloud.ID, tmdb.ID)
	m.OutboundHTTP = &http.Client{Transport: httpFixture(func(r *http.Request) (*http.Response, error) {
		body := `{"id":99,"name":"Owned Show","first_air_date":"2025-01-01","overview":"show overview"}`
		if strings.Contains(r.URL.Path, "/season/") {
			body = `{"season_number":1,"episodes":[{"id":991,"show_id":99,"season_number":1,"episode_number":1,"name":"Episode one","overview":"first plot","air_date":"2026-01-01","runtime":20},{"id":993,"show_id":99,"season_number":1,"episode_number":3,"name":"Episode three","overview":"third plot","air_date":"2026-01-03","runtime":21}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, cloud.ID)
	file, candidate := jobs.UUID(), jobs.UUID()
	db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes)VALUES($1,$2,'owned-file-id','/115/Owned/combined.mkv',1)`, file, cloud.ID)
	db.Exec(ctx, `INSERT INTO library_candidates(id,file_id)VALUES($1,$2)`, candidate, file)
	e := Engine{DB: db, Connections: m, Observe: func(context.Context, string, string) (storage.File, error) {
		return storage.File{ID: "owned-file-id", Name: "combined.mkv", Path: "/115/Owned/combined.mkv", Size: 1, Hashes: map[uint32]string{}}, nil
	}}
	asset, err := e.Confirm(ctx, candidate, Confirmation{MediaType: "series", TMDBID: 99, Season: 1, Episodes: []int{3, 1}, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	var raw, show []byte
	rows, err := db.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, asset)
	if err != nil {
		t.Fatal(err)
	}
	outputs := []string{}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		outputs = append(outputs, p)
	}
	rows.Close()
	if len(outputs) != 2 || !strings.Contains(outputs[0], "S01E01") || !strings.Contains(outputs[1], "S01E03") || strings.Contains(outputs[0], "E03") {
		t.Fatal("Emby projection did not preserve sparse coverage", outputs)
	}
	output = outputs[0]
	first, _ := os.ReadFile(filepath.Join(root, outputs[0]))
	second, _ := os.ReadFile(filepath.Join(root, outputs[1]))
	if string(first) != string(second) {
		t.Fatal("same source has different fixed URI")
	}
	db.QueryRow(ctx, `SELECT media_identity FROM media_assets WHERE id=$1`, asset).Scan(&raw)
	db.QueryRow(ctx, `SELECT m.metadata FROM media_items m JOIN media_asset_bindings b ON b.media_id=m.id WHERE b.asset_id=$1`, asset).Scan(&show)
	if !strings.Contains(string(raw), "Episode one") || !strings.Contains(string(raw), "Episode three") || strings.Contains(string(show), "episodeDetails") {
		t.Fatal("asset episode context contaminated shared show")
	}
	var episodes []int
	var covered []byte
	db.QueryRow(ctx, `SELECT episodes FROM media_asset_bindings WHERE asset_id=$1`, asset).Scan(&covered)
	json.Unmarshal(covered, &episodes)
	if len(episodes) != 2 || episodes[0] != 1 || episodes[1] != 3 {
		t.Fatal("binding coverage lost")
	}
	nfoRaw, err := os.ReadFile(filepath.Join(root, strings.TrimSuffix(output, ".strm")+".nfo"))
	if err != nil {
		t.Fatal(err)
	}
	var n nfo
	xml.Unmarshal(nfoRaw, &n)
	if n.Title != "Episode one" || n.Runtime != nil || !strings.Contains(n.Plot, "整个原文件") || !strings.Contains(n.Plot, "1,3") {
		t.Fatal("alias metadata implies a false segment", n)
	}
	third, _ := os.ReadFile(filepath.Join(root, strings.TrimSuffix(outputs[1], ".strm")+".nfo"))
	xml.Unmarshal(third, &n)
	if n.Title != "Episode three" || *n.Episode != 3 || strings.Contains(n.Plot, "first plot") {
		t.Fatal("per-entry episode metadata is wrong", n)
	}
	// Wrong season coverage is a business error before artifact/grant mutation.
	before := asset
	_, err = e.Confirm(ctx, candidate, Confirmation{MediaType: "series", TMDBID: 99, Season: 1, Episodes: []int{2}, Revision: 1})
	if err == nil {
		t.Fatal("missing provider episode silently supplied")
	}
	var current string
	db.QueryRow(ctx, `SELECT asset_id::text FROM library_candidates WHERE id=$1`, candidate).Scan(&current)
	if current != before {
		t.Fatal("failed request mutated existing candidate")
	}
}
