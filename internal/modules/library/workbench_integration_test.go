package library

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedHistoryFiltersAndManagedOutputEvidence(t *testing.T) {
	ctx, e, c := scanWorkbenchFixture(t)
	e.DB.Exec(ctx, `UPDATE application_settings SET value=jsonb_set(value,'{embyUserId}','""') WHERE singleton`)
	scan, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, total, err := e.History(ctx, HistoryFilter{Kind: "scan", Limit: 10})
	if err != nil || total != 1 || rows[0].ID != scan {
		t.Fatal(rows, total, err)
	}
	_, total, err = e.History(ctx, HistoryFilter{Kind: "scan", State: "succeeded", Limit: 10})
	if err != nil || total != 0 {
		t.Fatal(total, err)
	}
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, c)
	file, err := e.indexFile(ctx, c, storage.File{ID: "owned", Name: "owned.mkv", Path: "/115/Media/owned.mkv", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id,media_identity)VALUES($1,$1,'{}')`, file)
	e.DB.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'Movies/Owned/owned.strm','owned')`, jobs.UUID(), file)
	items, total, err := e.ManagedAssets(ctx, 10, 0, "owned")
	if err != nil || total != 1 || items[0].EmbyState != "pending" || !strings.Contains(string(items[0].Outputs), "owned.strm") {
		t.Fatal(items, total, err)
	}
}
func TestManualEmbyTargetRefreshRequiresExactPathsAndUniqueIdentity(t *testing.T) {
	ctx, e, c := scanWorkbenchFixture(t)
	e.DB.Exec(ctx, `UPDATE application_settings SET value=jsonb_set(value,'{embyUserId}','""') WHERE singleton`)
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, c)
	file, err := e.indexFile(ctx, c, storage.File{ID: "owned", Name: "owned.mkv", Path: "/115/Media/owned.mkv", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id,media_identity)VALUES($1,$1,'{}')`, file)
	media := jobs.UUID()
	e.DB.QueryRow(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',101,'Owned','{}')ON CONFLICT(media_type,tmdb_id)DO UPDATE SET title=excluded.title RETURNING id::text`, media).Scan(&media)
	e.DB.Exec(ctx, `INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, file, media)
	e.DB.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'Movies/Owned/owned.strm','owned')`, jobs.UUID(), file)
	output := path.Join("/emby/strm", "Movies/Owned/owned.strm")
	visible := false
	duplicate := false
	refreshed := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "owned-emby-key" {
			t.Error("missing scoped key")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/Library/Refresh" {
			t.Error("broad refresh used")
			w.WriteHeader(500)
			return
		}
		if r.URL.Path == "/Library/Media/Updated" {
			var payload struct{ Updates []struct{ Path string } }
			if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Updates) != 1 || payload.Updates[0].Path != output {
				t.Error("wrong refresh path")
			}
			refreshed++
			w.WriteHeader(204)
			return
		}
		if r.URL.Path != "/Items" {
			t.Error(r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if !visible {
			io.WriteString(w, `{"Items":[],"TotalRecordCount":0}`)
			return
		}
		item := map[string]any{"Id": "owned-item", "Path": output, "ProviderIds": map[string]string{"Tmdb": "101"}}
		items := []any{item}
		if duplicate {
			items = append(items, map[string]any{"Id": "duplicate", "Path": output})
		}
		json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
	}))
	defer srv.Close()
	emby, err := e.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "emby", Name: "owned emby", Enabled: true, Settings: map[string]any{"endpoint": srv.URL, "strmPathPrefix": "/emby/strm"}, Secrets: map[string]string{"apiKey": "owned-emby-key"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = emby
	payload, _ := json.Marshal(map[string]string{"assetId": file})
	job := jobs.Job{ID: jobs.UUID(), Payload: payload}
	if err = e.Refresh(ctx, job); err != nil || refreshed != 1 {
		t.Fatal(err, refreshed)
	}
	if err = e.CheckEmby(ctx, job); err == nil {
		t.Fatal("refresh receipt counted as visible")
	}
	var state string
	e.DB.QueryRow(ctx, `SELECT state FROM library_emby_checks WHERE asset_id=$1`, file).Scan(&state)
	if state != "pending" {
		t.Fatal(state)
	}
	visible = true
	duplicate = true
	if err = e.CheckEmby(ctx, job); err == nil {
		t.Fatal("ambiguous Emby paths accepted")
	}
	duplicate = false
	if err = e.CheckEmby(ctx, job); err != nil {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT state FROM library_emby_checks WHERE asset_id=$1`, file).Scan(&state)
	if state != "visible" {
		t.Fatal(state)
	}
	var mapped int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM emby_item_mappings WHERE asset_id=$1`, file).Scan(&mapped)
	if mapped != 1 {
		t.Fatal(mapped)
	}
	if err = e.CheckEmby(ctx, job); err != nil {
		t.Fatal("idempotent visibility", err)
	}
	e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, emby.ID)
	listed, _, err := e.ManagedAssets(ctx, 10, 0, "")
	if err != nil || listed[0].EmbyState != "pending" || string(listed[0].EmbyPaths) != "[]" {
		t.Fatal("stale connection visibility shown as current", listed, err)
	}

}

func TestManagedContentReturnsOriginalResolverOnlyAfterOwnershipAndDigestProof(t *testing.T) {
	ctx, e, c := scanWorkbenchFixture(t)
	e.Connections.Config.STRMOutputDir = t.TempDir()
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, c)
	file, err := e.indexFile(ctx, c, storage.File{ID: "owned", Name: "owned.mkv", Path: "/115/Media/owned.mkv", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$1)`, file)
	j := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	original := "https://owned.fixture/api/v1/cloudStorage/strm302?fileKey=original-owned-key\n"
	if err = j.Write(ctx, e.Connections.Config.STRMOutputDir, "Movies/Owned.strm", file, "asset:"+file, "strm", []byte(original)); err != nil {
		t.Fatal(err)
	}
	proof, err := e.OutputContent(ctx, file, "Movies/Owned.strm")
	if err != nil || proof.Content != original {
		t.Fatal(proof, err)
	}
	if _, err = e.OutputContent(ctx, jobs.UUID(), "Movies/Owned.strm"); err == nil {
		t.Fatal("other asset read body")
	}
	os.WriteFile(filepath.Join(e.Connections.Config.STRMOutputDir, "Movies/Owned.strm"), []byte("handmade replacement"), 0600)
	if _, err = e.OutputContent(ctx, file, "Movies/Owned.strm"); err == nil {
		t.Fatal("handmade changed body returned as owned")
	}
	if _, err = e.OutputContent(ctx, file, "../password"); err == nil {
		t.Fatal("escaped output path")
	}
}

func TestSelectedScrapeOutputsAndHistoryRemovalPreserveEvidence(t *testing.T) {
	ctx, e, c := scanWorkbenchFixture(t)
	e.Connections.Config.STRMOutputDir = t.TempDir()
	e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, c)
	file, err := e.indexFile(ctx, c, storage.File{ID: "owned", Name: "owned.mkv", Path: "/115/Media/owned.mkv", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$1)`, file)
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	policy := metadata.DefaultScrapePolicy()
	policy.Movie.NFO = false
	policy.Movie.Poster = true
	item := metadata.Item{ID: 1, Title: "Owned", MediaType: "movie", Scrape: &metadata.ScrapeBundle{Policy: policy, Artworks: []metadata.Artwork{{Level: "movie", Role: "poster", Extension: ".png", Data: imageData.Bytes()}}}}
	j := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := OutputProjection{Path: "Movies/Owned/Owned [owned].strm", WholeFilePlayback: true}
	intents, err := StageOutputProjections(ctx, j, tx, e.Connections.Config.STRMOutputDir, file, item, nil, []OutputProjection{p}, []byte("https://owned.fixture/original\n"), "owned")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range intents {
		if err = j.Apply(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(filepath.Join(e.Connections.Config.STRMOutputDir, "Movies/Owned/Owned [owned].nfo")); !os.IsNotExist(err) {
		t.Fatal("disabled NFO generated", err)
	}
	if _, err = os.Stat(filepath.Join(e.Connections.Config.STRMOutputDir, "Movies/Owned/Owned [owned]-poster.png")); err != nil {
		t.Fatal("selected poster not generated", err)
	}
	scan, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := e.ScanByID(ctx, scan)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.HideHistory(ctx, []HistorySelection{{Kind: "scan", ID: scan, UpdatedAt: s.UpdatedAt}}, true); err == nil {
		t.Fatal("active scan hidden")
	}
	e.DB.Exec(ctx, `UPDATE library_scans SET state='succeeded',updated_at=now() WHERE id=$1`, scan)
	s, _ = e.ScanByID(ctx, scan)
	if err = e.HideHistory(ctx, []HistorySelection{{Kind: "scan", ID: scan, UpdatedAt: s.UpdatedAt}}, true); err != nil {
		t.Fatal(err)
	}
	_, count, err := e.History(ctx, HistoryFilter{Kind: "scan", Limit: 10})
	if err != nil || count != 0 {
		t.Fatal("hidden history still listed", count, err)
	}
	var retained bool
	e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_scans WHERE id=$1)`, scan).Scan(&retained)
	if !retained {
		t.Fatal("recovery evidence deleted")
	}
	e.DB.Exec(ctx, `UPDATE library_scans SET updated_at=now()+interval '1 second',message='new evidence' WHERE id=$1`, scan)
	_, count, err = e.History(ctx, HistoryFilter{Kind: "scan", Limit: 10})
	if err != nil || count != 1 {
		t.Fatal("new evidence hidden by old deletion", count, err)
	}
}
