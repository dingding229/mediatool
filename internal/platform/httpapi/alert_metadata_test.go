package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/database"
	"net/http"
	"os"
	"testing"
)

func TestOpsAlertsExposeCurrentReadOnlyMetadata(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("test database required", name, err)
	}
	var original []byte
	if err = db.QueryRow(ctx, `SELECT value FROM ops_policy WHERE singleton`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `UPDATE ops_policy SET value=$1 WHERE singleton`, original)
	defer db.Exec(ctx, `DELETE FROM ops_alerts`)
	if _, err = db.Exec(ctx, `DELETE FROM ops_alerts;INSERT INTO ops_alerts(code)VALUES('disk'),('failures'),('load'),('memory'),('plugin_journal'),('workers')`); err != nil {
		t.Fatal(err)
	}
	h := New(cfg(), Dependencies{DB: db, AuthSessions: testSessionReader{}})
	for _, memory := range []float64{90, 76} {
		p := maintenance.Defaults()
		p.MemoryPercent = memory
		p.DiskFreeGB = 9
		p.FailedJobs = 4
		p.LoadPerCPU = 1.25
		raw, _ := json.Marshal(p)
		if _, err = db.Exec(ctx, `UPDATE ops_policy SET value=$1 WHERE singleton`, raw); err != nil {
			t.Fatal(err)
		}
		w := invoke(h, http.MethodGet, "/api/v1/ops", "", cfg().SessionKey)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var got struct {
			Data struct{ Alerts []maintenance.Alert }
		}
		if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Data.Alerts) != 6 {
			t.Fatal(got)
		}
		expected := map[string]struct {
			n                float64
			unit, comparison string
		}{"memory": {memory, "percent", ">="}, "disk": {9, "GiB", "<="}, "load": {1.25, "load_per_cpu", ">="}, "failures": {4, "jobs_per_10m", ">="}, "workers": {maintenance.OfflineQueueAlertThreshold, "queues", ">="}, "plugin_journal": {maintenance.PluginJournalAlertThreshold, "entries", ">="}}
		for _, a := range got.Data.Alerts {
			want, ok := expected[a.Code]
			if !ok || a.Threshold == nil || *a.Threshold != want.n || a.Unit != want.unit || a.Comparison != want.comparison {
				t.Fatalf("%+v want %+v", a, want)
			}
			if a.State != "normal" || a.Hits != 0 || a.AcknowledgedAt != nil {
				t.Fatal("metadata read changed state", a)
			}
		}
	}
}
