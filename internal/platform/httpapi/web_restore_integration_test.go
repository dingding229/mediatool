package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/database"
)

func TestWebRestoreRecentSessionCSRFAndMaintenanceGate(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
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
	if _, err = db.Exec(ctx, `TRUNCATE administrator_accounts,administrator_auth_limits,ops_web_restores,ops_backups CASCADE`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `TRUNCATE administrator_accounts,administrator_auth_limits,ops_web_restores,ops_backups CASCADE`)
	token, _ := (auth.Auth{Key: cfg().SessionKey}).Issue(time.Now())
	if _, err = (accounts.Repository{DB: db}).Register(ctx, "b4_admin", "owned-password", token); err != nil {
		t.Fatal(err)
	}
	backupID := "11111111-0000-4000-8000-000000000075"
	restoreID := "22222222-0000-4000-8000-000000000075"
	_, err = db.Exec(ctx, `INSERT INTO ops_backups(id,state)VALUES($1,'ready')`, backupID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO ops_web_restores(id,backup_id,manifest_digest,protected)VALUES($1,$2,'digest','{}')`, restoreID, backupID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(cfg(), Dependencies{DB: db})
	call := func(method, path, body string, csrf, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		if cookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/ops/restores/" + restoreID + "/execute"
	raw, _ := json.Marshal(map[string]string{"digest": "digest", "text": "恢复 " + backupID})
	if w := call("GET", "/api/v1/ops/restores", "", false, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call("POST", path, string(raw), false, true); w.Code != 403 {
		t.Fatal("CSRF accepted", w.Code, w.Body.String())
	}
	sum := sha256.Sum256([]byte(token))
	db.Exec(ctx, `UPDATE administrator_sessions SET created_at=now()-interval '11 minutes',last_active_at=now() WHERE token_digest=$1`, sum[:])
	if w := call("POST", path, string(raw), true, true); w.Code != 403 || !strings.Contains(w.Body.String(), "recent_login_required") {
		t.Fatal(w.Code, w.Body.String())
	}
	db.Exec(ctx, `UPDATE administrator_sessions SET created_at=now() WHERE token_digest=$1`, sum[:])
	if w := call("POST", path, string(raw), true, true); w.Code != 202 || !strings.Contains(w.Body.String(), "pending") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/v1/jobs", `{"kind":"system.check"}`, true, true); w.Code != 423 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/v1/ops/resume", `{"confirm":true}`, true, true); w.Code != 423 {
		t.Fatal("legacy bypass", w.Code)
	}
	if w := call("POST", "/emby/Sessions/Playing", `{}`, false, false); w.Code != 423 {
		t.Fatal("native bypass", w.Code)
	}
	if w := call("GET", "/api/v1/ops/restores", "", true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	loginBody := `{"username":"b4_admin","password":"owned-password"}`
	if w := call("POST", "/api/v1/auth/login", loginBody, true, false); w.Code != 423 {
		t.Fatal("login wrote auth during import", w.Code)
	}
	e := maintenance.Engine{DB: db}
	if err = e.Resume(ctx, 1); err == nil {
		t.Fatal("CLI resume bypass")
	}
	if _, err = db.Exec(ctx, `UPDATE ops_web_restores SET state='unknown' WHERE id=$1`, restoreID); err != nil {
		t.Fatal(err)
	}
	holder, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err = holder.Exec(ctx, `SET LOCAL application_name='mediatool-maintenance'`); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/v1/auth/login", loginBody, true, false); w.Code != 423 {
		t.Fatal("live import client bypassed", w.Code)
	}
	if err = holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/v1/auth/login", loginBody, true, false); w.Code != 200 {
		t.Fatal("quiescent unknown blocked fresh login", w.Code, w.Body.String())
	}

}
