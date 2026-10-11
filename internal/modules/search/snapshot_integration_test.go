package search

import (
	"context"
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

func snapshotSearchFixture(t *testing.T) (context.Context, Engine, string, func() string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,site_definitions,search_runs,download_tasks,outbox_events,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), cleanup) })
	vault, err := secrets.New(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	repo := integrations.Repository{DB: db, Vault: vault}
	account, err := repo.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned snapshot fixture", Enabled: true, Settings: map[string]any{"endpoint": "https://owned.invalid", "definitionId": "owned_snapshot"}})
	if err != nil {
		t.Fatal(err)
	}
	newRun := func() string {
		t.Helper()
		id := jobs.UUID()
		if _, err = db.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	return ctx, Engine{DB: db, Vault: vault}, account.ID, newRun
}

func TestPostgresSearchRunCannotReadAnotherRunsPromotionOrPeers(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	free, paid, noHR, hr := float64(0), float64(1), false, true
	first, second := newRun(), newRun()
	original := sites.Resource{TorrentID: "42", Title: "Owned Movie (2026)", Size: 1000, Seeders: 12, Leechers: 4, DownloadFactor: &free, HR: &noHR, DownloadURL: "https://owned.invalid/download.php?id=42&passkey=FIRST-KEY"}
	if err := engine.SaveResources(ctx, first, account, []sites.Resource{original}); err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Title, changed.Seeders, changed.Leechers, changed.DownloadFactor, changed.HR = "Changed elsewhere", 2, 1, &paid, &hr
	changed.DownloadURL = "https://owned.invalid/download.php?id=42&passkey=SECOND-KEY"
	if err := engine.SaveResources(ctx, second, account, []sites.Resource{changed}); err != nil {
		t.Fatal(err)
	}
	before, err := engine.Get(ctx, first, 1, 50)
	if err != nil || len(before.Resources) != 1 {
		t.Fatal("first run unavailable", err)
	}
	r := before.Resources[0]
	if r.Title != original.Title || r.Seeders != 12 || r.Leechers != 4 || r.DownloadFactor == nil || *r.DownloadFactor != 0 || r.Metadata["hr"] != false {
		t.Fatal("first run consumed another run's mutable title/peers/promotion/H&R")
	}
}

func TestPostgresSameRunRSSHydrationCannotUseOtherRunGlobalMetadata(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	free, paid, noHR, hr := float64(0), float64(1), false, true
	first, second := newRun(), newRun()
	full := sites.Resource{TorrentID: "42", Title: "Owned Movie (2026)", Size: 1000, Seeders: 12, Leechers: 4, DownloadFactor: &free, HR: &noHR}
	if err := engine.SaveResources(ctx, first, account, []sites.Resource{full}); err != nil {
		t.Fatal(err)
	}
	other := full
	other.Seeders, other.Leechers, other.DownloadFactor, other.HR = 2, 1, &paid, &hr
	if err := engine.SaveResources(ctx, second, account, []sites.Resource{other}); err != nil {
		t.Fatal(err)
	}
	partial := sites.Resource{TorrentID: "42", Title: full.Title, Partial: true, Seeders: -1, Leechers: -1}
	if err := engine.SaveResources(ctx, first, account, []sites.Resource{partial}); err != nil {
		t.Fatal(err)
	}
	run, err := engine.Get(ctx, first, 1, 50)
	if err != nil || len(run.Resources) != 1 {
		t.Fatal(err)
	}
	r := run.Resources[0]
	if r.Seeders != 12 || r.Leechers != 4 || r.DownloadFactor == nil || *r.DownloadFactor != 0 || r.Metadata["hr"] != false {
		t.Fatal("same-run RSS hydration inherited another run's global metadata")
	}
}

func TestPostgresSnapshotHintsAreRunScopedAndLegacyLinksRequireNewSearch(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	first, second, legacy := newRun(), newRun(), newRun()
	item := sites.Resource{TorrentID: "42", Title: "Owned", DownloadURL: "https://owned.invalid/download.php?id=42&passkey=FIRST-KEY"}
	if err := engine.SaveResources(ctx, first, account, []sites.Resource{item}); err != nil {
		t.Fatal(err)
	}
	run, err := engine.Get(ctx, first, 1, 50)
	if err != nil || len(run.Resources) != 1 {
		t.Fatal(err)
	}
	id := run.Resources[0].ID
	item.DownloadURL = "https://owned.invalid/download.php?id=42&passkey=SECOND-KEY"
	if err = engine.SaveResources(ctx, second, account, []sites.Resource{item}); err != nil {
		t.Fatal(err)
	}
	s, err := engine.snapshot(ctx, first, id)
	if err != nil {
		t.Fatal(err)
	}
	hint, err := engine.Vault.Open(snapshotAAD+s.ID, s.Hint)
	if err != nil || hint["url"] != "https://owned.invalid/download.php?id=42&passkey=FIRST-KEY" {
		t.Fatal("first hint overwritten", err)
	}
	other, err := engine.snapshot(ctx, second, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Vault.Open(snapshotAAD+other.ID, s.Hint); err == nil {
		t.Fatal("ciphertext copied to another snapshot identity was accepted")
	}
	if _, err = engine.DB.Exec(ctx, `INSERT INTO search_run_resources(run_id,resource_id)VALUES($1,$2)`, legacy, id); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Get(ctx, legacy, 1, 50); err != ErrSnapshotUnavailable {
		t.Fatal("legacy link silently used mutable current metadata", err)
	}
	if err = engine.ValidateRunResource(ctx, first, id); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.DB.Exec(ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err = engine.ValidateRunResource(ctx, first, id); err != ErrSnapshotExpired {
		t.Fatal("expired snapshot accepted", err)
	}
	if _, err = engine.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err = engine.ValidateRunResource(ctx, second, id); err != ErrSnapshotChanged {
		t.Fatal("changed account revision accepted", err)
	}
}

func TestPostgresSelectedSnapshotProofSurvivesOfflineMasterKeyRotation(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	runID := newRun()
	item := sites.Resource{TorrentID: "42", Title: "Owned", DownloadURL: "https://owned.invalid/download.php?id=42&passkey=FIXTURE-KEY"}
	if err := engine.SaveResources(ctx, runID, account, []sites.Resource{item}); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Get(ctx, runID, 1, 50)
	if err != nil || len(result.Resources) != 1 {
		t.Fatal(err)
	}
	id := result.Resources[0].ID
	original, err := engine.SourceSnapshotDigest(ctx, runID, id)
	if err != nil {
		t.Fatal(err)
	}
	next, err := secrets.New(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = secrets.Rekey(ctx, engine.DB, engine.Vault, next, false); err != nil {
		t.Fatal(err)
	}
	engine.Vault = next
	rotated, err := engine.SourceSnapshotDigest(ctx, runID, id)
	if err != nil || rotated != original {
		t.Fatal("re-encryption bytes invalidated unchanged source evidence", err)
	}
}
