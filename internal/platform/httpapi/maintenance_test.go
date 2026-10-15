package httpapi

import (
	"context"
	"media-tool/internal/platform/database"
	"net/http"
	"os"
	"testing"
)

func TestMaintenanceAdminBoundaries(t *testing.T) {
	h, _ := testHandler(nil)
	for _, path := range []string{"/api/v1/ops", "/api/v1/ops/trends?hours=24", "/api/v1/ops/backups/00000000-0000-4000-8000-000000000001/download"} {
		if w := invoke(h, http.MethodGet, path, "", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := invoke(h, http.MethodGet, "/api/v1/ops", "", cfg().SessionKey)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
func TestMaintenanceWriteGateAndPolicyRevision(t *testing.T) {
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
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	db.Exec(ctx, `UPDATE ops_mode SET paused=true WHERE singleton`)
	defer db.Exec(ctx, `UPDATE ops_mode SET paused=false,reason='' WHERE singleton`)
	h := New(cfg(), Dependencies{DB: db, AuthSessions: testSessionReader{}})
	if w := invoke(h, http.MethodPost, "/api/v1/jobs", `{"kind":"system.check"}`, cfg().SessionKey); w.Code != 423 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := invoke(h, http.MethodGet, "/api/v1/ops", "", cfg().SessionKey); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := invoke(h, http.MethodPost, "/api/v1/ops/backups", `{"confirm":false}`, cfg().SessionKey); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := invoke(h, http.MethodGet, "/api/v1/ops/trends?hours=999999", "", cfg().SessionKey); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
