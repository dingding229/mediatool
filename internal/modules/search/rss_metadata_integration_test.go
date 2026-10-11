package search

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgresRSSHydratesOnlyFromMetadataInCurrentRun(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,site_definitions,search_runs,download_tasks,outbox_events,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	vault, err := secrets.New(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	repository := integrations.Repository{DB: db, Vault: vault}
	account, err := repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned metadata fixture", Enabled: true, Settings: map[string]any{"endpoint": "https://owned.invalid", "definitionId": "owned_feed"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Vault: vault}
	newRun := func() string {
		t.Helper()
		id := jobs.UUID()
		if _, err = db.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	free, noHR := float64(0), false
	full := sites.Resource{TorrentID: "42", Title: "Owned Movie (2026)", Size: 2147483648, Seeders: 12, Leechers: 4, DownloadFactor: &free, HR: &noHR, Labels: []string{"full-search-label"}, DownloadURL: "https://owned.invalid/download.php?id=42&passkey=FIXTURE-KEY"}
	partial := sites.Resource{TorrentID: "42", Title: full.Title, Partial: true, Seeders: -1, Leechers: -1, DownloadURL: "https://owned.invalid/torrent/download/42.NEXT-FIXTURE-KEY"}
	read := func() (Resource, []byte) {
		t.Helper()
		var r Resource
		var meta, encrypted []byte
		if err = db.QueryRow(ctx, `SELECT id::text,size_bytes,seeders,leechers,download_factor,public_metadata,download_hint_ciphertext FROM resources WHERE account_id=$1 AND torrent_id='42'`, account.ID).Scan(&r.ID, &r.Size, &r.Seeders, &r.Leechers, &r.DownloadFactor, &meta, &encrypted); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(meta, &r.Metadata); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(meta), "FIXTURE-KEY") || strings.Contains(string(encrypted), "FIXTURE-KEY") {
			t.Fatal("RSS download key in public metadata or plaintext")
		}
		return r, encrypted
	}
	firstRun := newRun()
	if err = engine.SaveResources(ctx, firstRun, account.ID, []sites.Resource{full, partial}); err != nil {
		t.Fatal(err)
	}
	current, _ := read()
	if current.Size != full.Size || current.Seeders != 12 || current.Leechers != 4 || current.DownloadFactor == nil || *current.DownloadFactor != 0 || current.Metadata["hr"] != false {
		t.Fatal("RSS discarded same-run confirmed search metadata")
	}
	secondRun := newRun()
	if err = engine.SaveResources(ctx, secondRun, account.ID, []sites.Resource{partial}); err != nil {
		t.Fatal(err)
	}
	unknown, encrypted := read()
	if unknown.Size != 0 || unknown.Seeders != 0 || unknown.Leechers != 0 || unknown.DownloadFactor != nil || unknown.Metadata["hr"] != nil || unknown.Metadata["labels"] != nil {
		t.Fatal("RSS-only evaluation inherited stale size/peers/free/H&R/labels")
	}
	hint, err := vault.Open(unknown.ID, encrypted)
	if err != nil || hint["url"] != partial.DownloadURL {
		t.Fatal("current keyed download hint lost", err)
	}
	// Explicit RSS values are current evidence even without a same-run search.
	thirdRun := newRun()
	partial.Size, partial.DownloadFactor, partial.HR = 1024, &free, &noHR
	if err = engine.SaveResources(ctx, thirdRun, account.ID, []sites.Resource{partial}); err != nil {
		t.Fatal(err)
	}
	observed, _ := read()
	if observed.Size != 1024 || observed.DownloadFactor == nil || *observed.DownloadFactor != 0 || observed.Metadata["hr"] != false || observed.Seeders != 0 {
		t.Fatal("explicit RSS evidence not stored")
	}
}
