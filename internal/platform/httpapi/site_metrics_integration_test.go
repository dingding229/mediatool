package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresSiteMetricsKeepLastSuccessAndRejectChangedOrOlderCollectors(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated postgres required")
	}
	db.Exec(ctx, "TRUNCATE integrations,site_definitions CASCADE")
	defer db.Exec(ctx, "TRUNCATE integrations,site_definitions CASCADE")
	if _, e = db.Exec(ctx, `INSERT INTO site_definitions(id,name,source_commit,definition)VALUES('owned_metric','Owned','owned','{}')`); e != nil {
		t.Fatal(e)
	}
	v, _ := secrets.New(strings.Repeat("7", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	account, e := repo.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned", Enabled: true, Settings: map[string]any{"definitionId": "owned_metric", "endpoint": "https://owned.invalid"}})
	if e != nil {
		t.Fatal(e)
	}
	manager := integrations.NewManager(repo, cfg())
	defer manager.Close()
	api := &API{Deps: Dependencies{DB: db, Integrations: manager}}
	definition, e := (sites.Repository{DB: db}).Get(ctx, "owned_metric")
	if e != nil {
		t.Fatal(e)
	}
	fp := sites.DefinitionFingerprint(definition.RawJSON, definition.CommonJSON, definition.SourceCommit)
	uploaded := int64(100)
	metric := sites.Metrics{Uploaded: &uploaded, Partial: true, Notes: []string{}}
	started := time.Now().Add(-time.Minute)
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision, fp, started, metric, nil); e != nil {
		t.Fatal(e)
	}
	var snapshot string
	db.QueryRow(ctx, "SELECT metrics::text FROM site_metrics WHERE account_id=$1", account.ID).Scan(&snapshot)
	if !strings.Contains(snapshot, `"uploaded": 100`) {
		t.Fatal(snapshot)
	}
	uploaded = 999
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision, fp, started.Add(-time.Minute), metric, nil); e != nil {
		t.Fatal(e)
	}
	var unchanged string
	db.QueryRow(ctx, "SELECT metrics::text FROM site_metrics WHERE account_id=$1", account.ID).Scan(&unchanged)
	if unchanged != snapshot {
		t.Fatal("older collection replaced new data")
	}
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision, fp, time.Now(), metric, errors.New("failed")); e != nil {
		t.Fatal(e)
	}
	db.QueryRow(ctx, "SELECT metrics::text FROM site_metrics WHERE account_id=$1", account.ID).Scan(&unchanged)
	if unchanged != snapshot {
		t.Fatal("failure destroyed successful snapshot")
	}
	w := httptest.NewRecorder()
	api.siteMetrics(w, httptest.NewRequest("GET", "/", nil))
	var response struct {
		Data []struct {
			Stale bool `json:"stale"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 200 || len(response.Data) != 1 || !response.Data[0].Stale {
		t.Fatal(w.Code, w.Body.String())
	}
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision+1, fp, time.Now(), metric, nil); !errors.Is(e, sitemetrics.ErrChanged) {
		t.Fatal("revision change", e)
	}
	db.Exec(ctx, "UPDATE site_definitions SET source_commit='new' WHERE id='owned_metric'")
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision, fp, time.Now(), metric, nil); !errors.Is(e, sitemetrics.ErrChanged) {
		t.Fatal("definition change", e)
	}
	db.Exec(ctx, "UPDATE integrations SET enabled=false WHERE id=$1", account.ID)
	if e = (sitemetrics.Engine{DB: db, Connections: api.Deps.Integrations}).Save(ctx, account.ID, account.Revision, "", time.Now(), metric, nil); !errors.Is(e, sitemetrics.ErrChanged) {
		t.Fatal("disabled", e)
	}
}
