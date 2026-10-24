package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresPreflightUsesDatabaseAndDisabledProfileBlocksEnvironmentFallback(t *testing.T) {
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
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated test DB required")
	}
	cleanup := `TRUNCATE integrations,cloud_files,storage_connections,media_assets CASCADE`
	db.Exec(ctx, cleanup)
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	c := cfg()
	c.STRMOutputDir = t.TempDir()
	c.EmbyUpstream = "http://127.0.0.1:9"
	c.EmbyProxyEnabled = true
	c.Emby302Enabled = true
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, c)
	defer manager.Close()
	_, err = repo.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "owned", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "PRIVATE-PROVIDER"}})
	if err != nil {
		t.Fatal(err)
	}
	input := integrations.Input{Kind: "emby", Name: "owned emby", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "strmPathPrefix": "/strm", "gatewayEnabled": true}, Secrets: map[string]string{"apiKey": "PRIVATE-EMBY"}}
	record, err := repo.Save(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	h := New(c, Dependencies{AuthSessions: testSessionReader{}, DB: db, Integrations: manager, Logger: slog.Default()})
	w := invoke(h, "GET", "/api/v1/diagnostics/playback/preflight", "", c.SessionKey)
	if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response.Data["cloudSource"] != "database" || response.Data["embySource"] != "database" || response.Data["gatewayEnabled"] != true {
		t.Fatal(response)
	}

	t.Run("file grant rotation is journaled and manual edits survive", func(t *testing.T) {
		storageID, fileID, assetID, entryID := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
		if _, err = db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned fixture','clouddrive2')`, storageID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/Media/fixture.mkv',1)`, fileID, storageID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, assetID, fileID); err != nil {
			t.Fatal(err)
		}
		service := playbackgrants.Service{DB: db, Vault: vault, PublicURL: c.PublicURL}
		tx, _ := db.Begin(ctx)
		oldLink, grantID, err := service.Prepare(ctx, tx, assetID, "/115/Media/fixture.mkv")
		if err != nil {
			t.Fatal(err)
		}
		output := "Movies/fixture.strm"
		if _, err = tx.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)`, entryID, assetID, output, playbackgrants.SecretFreeKeyRef(grantID)); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = strm.WriteAtomic(c.STRMOutputDir, output, oldLink); err != nil {
			t.Fatal(err)
		}
		if result := invoke(h, "GET", "/api/v1/media/assets/"+assetID+"/strm", "", c.SessionKey); result.Code != 404 {
			t.Fatal("removed management STRM route remains", result.Code)
		}
		if result := invoke(h, "POST", "/api/v1/media/assets/"+assetID+"/grant/revoke", "{}", c.SessionKey); result.Code != 200 {
			t.Fatal("revoke failed", result.Code)
		}
		check := &API{Deps: Dependencies{DB: db, Integrations: manager}, Config: c, Builder: strm.Builder{BaseURL: c.PublicURL}}
		if !check.gateway(ctx).MetadataLink(ctx, oldLink) {
			t.Fatal("revoked file key would leak from metadata")
		}
		if result := invoke(h, "POST", "/api/v1/media/assets/"+assetID+"/grant/rotate", "{}", c.SessionKey); result.Code != 200 {
			t.Fatal("rotation failed", result.Code, result.Body.String())
		}
		previous, _ := os.ReadFile(filepath.Join(c.STRMOutputDir, output))
		if strings.TrimSpace(string(previous)) == oldLink {
			t.Fatal("rotation did not rewrite STRM")
		}
		if err = os.WriteFile(filepath.Join(c.STRMOutputDir, output), []byte("manual edit"), 0640); err != nil {
			t.Fatal(err)
		}
		if result := invoke(h, "POST", "/api/v1/media/assets/"+assetID+"/grant/rotate", "{}", c.SessionKey); result.Code != 409 {
			t.Fatal("manual edit overwritten", result.Code)
		}
		raw, _ := os.ReadFile(filepath.Join(c.STRMOutputDir, output))
		if string(raw) != "manual edit" {
			t.Fatal("manual edit lost")
		}
		var repairID string
		if err = db.QueryRow(ctx, `SELECT id::text FROM media_artifact_journal WHERE asset_id=$1 AND state='conflict'`, assetID).Scan(&repairID); err != nil {
			t.Fatal("rotation repair intent missing", err)
		}
		if err = os.WriteFile(filepath.Join(c.STRMOutputDir, output), previous, 0640); err != nil {
			t.Fatal(err)
		}
		if result := invoke(h, "POST", "/api/v1/library/artifact-repairs/"+repairID+"/retry", "{}", c.SessionKey); result.Code != 200 {
			t.Fatal("repair failed", result.Code)
		}
		raw, _ = os.ReadFile(filepath.Join(c.STRMOutputDir, output))
		u, _ := url.Parse(strings.TrimSpace(string(raw)))
		if _, err = service.Authorize(ctx, assetID, u.Query().Get("apiKey")); err != nil {
			t.Fatal("repaired STRM key mismatched")
		}
		if _, err = db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`); err != nil {
			t.Fatal(err)
		}
	})
	input.Enabled = false
	input.Revision = record.Revision
	if _, err = repo.Save(ctx, record.ID, input); err != nil {
		t.Fatal(err)
	}
	w = invoke(h, "GET", "/api/v1/diagnostics/playback/preflight", "", c.SessionKey)
	json.Unmarshal(w.Body.Bytes(), &response)
	if response.Data["embyConfigured"] != false {
		t.Fatal("disabled DB profile fell back to env")
	}
	w = invoke(h, "GET", "/emby/System/Info/Public", "", "")
	if w.Code != 503 {
		t.Fatal("disabled gateway still routed", w.Code)
	}
	if w = invoke(h, "GET", "/api/v1/media/assets?page=1&pageSize=101", "", c.SessionKey); w.Code != 400 {
		t.Fatal("unbounded pagination")
	}
	if w = invoke(h, "GET", "/api/v1/media/assets?page=1&pageSize=50", "", c.SessionKey); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("paged DTO", w.Code, w.Body.String())
	}
}
