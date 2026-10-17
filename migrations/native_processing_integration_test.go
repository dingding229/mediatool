package migrations_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeProcessingAdminRealHTTPSettingsRetryGCAndBoundaries(t *testing.T) {
	f := newStillFixture(t)
	service, root := stillRuntime(t, f)
	c := config.Config{PublicURL: "http://example.com", SessionKey: strings.Repeat("a", 64), STRMKey: strings.Repeat("b", 64), STRMAllowedRoots: []string{"/115"}}
	h := httpapi.New(c, httpapi.Dependencies{DB: f.pool, NativeStills: service, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var cookie *http.Cookie
	call := func(method, path string, in any, authenticated, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		if in != nil {
			if err := json.NewEncoder(&body).Encode(in); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &body)
		if authenticated && cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := call("POST", "/api/v1/auth/register", map[string]string{"username": "processing_admin", "password": "Admin26Pass", "confirmPassword": "Admin26Pass"}, false, true)
	check(w, 201)
	cookie = w.Result().Cookies()[0]
	base := "/api/v1/media-server/native/processing"
	for _, v := range []struct{ method, path string }{{"GET", base}, {"PUT", base + "/settings"}, {"GET", base + "/tasks"}, {"POST", base + "/retry"}, {"POST", base + "/gc"}} {
		check(call(v.method, v.path, nil, false, true), 401)
		if v.method != "GET" {
			check(call(v.method, v.path, nil, true, false), 403)
		}
	}
	var summary struct {
		Data admin.Summary `json:"data"`
	}
	w = call("GET", base, nil, true, false)
	check(w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	settings := summary.Data.Settings
	if !settings.Enabled || len(summary.Data.Probe) != 5 || len(summary.Data.Still) != 6 || summary.Data.Capacity.LimitBytes != 2<<30 {
		t.Fatal(summary)
	}
	settings.Enabled = false
	check(call("PUT", base+"/settings", settings, true, true), 200)
	check(call("PUT", base+"/settings", settings, true, true), 409)
	check(call("POST", base+"/retry", admin.Retry{Kind: "probe", ID: f.source.asset}, true, true), 503)
	settings.Enabled = true
	settings.Revision++
	check(call("PUT", base+"/settings", settings, true, true), 200)
	f.exec(t, `UPDATE media_probe_results SET state='needs_action',last_error_class='manual',last_error_reason='authorization',attempts=2 WHERE cloud_file_id=$1`, f.source.file)
	w = call("GET", base+"/tasks?kind=probe&state=needs_action&page=1&pageSize=1", nil, true, false)
	check(w, 200)
	var page struct {
		Data admin.Page `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Data.Total != 1 || len(page.Data.Items) != 1 || page.Data.Items[0].ID != f.source.asset || page.Data.Items[0].ErrorReason != "authorization" || !strings.Contains(page.Data.Items[0].Name, "S0E1") {
		t.Fatal(w.Body.String(), err)
	}
	check(call("GET", base+"/tasks?kind=probe&state=needs_action&page=2&pageSize=1", nil, true, false), 200)
	input := admin.Retry{Kind: "probe", State: "needs_action", Bulk: true}
	check(call("POST", base+"/retry", input, true, true), 400)
	input.Confirmed = true
	check(call("POST", base+"/retry", input, true, true), 202)
	if f.text(t, `SELECT state FROM media_probe_results WHERE cloud_file_id=$1`, f.source.file) != "pending" {
		t.Fatal("manual probe recovery")
	}
	f.enqueue(t, false)
	id := strconv.FormatInt(f.itemID, 10)
	check(call("POST", base+"/retry", admin.Retry{Kind: "still", ID: id}, true, true), 202)
	lease := rawStillClaim(t, f)
	image, err := service.Publish(f.ctx, lease, stillJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := f.store.Succeed(f.ctx, lease, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	raw, err := stillstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := raw.Publish(f.ctx, "00000000-0000-4000-8000-000000000026", strings.Repeat("b", 64), stillJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	if err = os.Chtimes(filepath.Join(root, orphan.RelativePath), old, old); err != nil {
		t.Fatal(err)
	}
	w = call("POST", base+"/gc", nil, true, true)
	check(w, 200)
	var gc struct {
		Data struct {
			Scanned, Reclaimed int
			ReclaimedBytes     int64
		}
	}
	if err = json.Unmarshal(w.Body.Bytes(), &gc); err != nil || gc.Data.Scanned > 200 || gc.Data.Reclaimed != 1 || gc.Data.ReclaimedBytes <= 0 {
		t.Fatal(w.Body.String(), err)
	}
	if _, err = os.Stat(filepath.Join(root, image.RelativePath)); err != nil {
		t.Fatal("referenced image lost", err)
	}
	if _, err = os.Stat(filepath.Join(root, orphan.RelativePath)); !os.IsNotExist(err) {
		t.Fatal("orphan survived", err)
	}
	for _, action := range []string{"native.processing.settings", "native.processing.retry", "native.probe.retry", "native.still.retry", "native.processing.gc.request", "native.processing.gc.result"} {
		if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action=$1`, action) < 1 {
			t.Fatal("missing audit", action)
		}
	}
	media, err := httpapi.NativeMediaHandler(c, httpapi.Dependencies{DB: f.pool})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{base, base + "/tasks", base + "/settings", base + "/retry", base + "/gc"} {
		for _, method := range []string{"GET", "PUT", "POST", "HEAD"} {
			r := httptest.NewRequest(method, path, nil)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			media.ServeHTTP(w, r)
			if w.Code != 404 {
				t.Fatal("admin port leak", method, path, w.Code)
			}
		}
	}
}
func TestNativeProcessingBatchLimitAndExactFilter(t *testing.T) {
	f := newStillFixture(t)
	media := f.media(t, 260001, "Synthetic Bulk Movie", "2026", `{}`)
	// One transaction seeds only the disposable media_tool_test schema.
	var n int
	err := f.db.QueryRowContext(f.ctx, `WITH seed AS(SELECT gen_random_uuid() id,g FROM generate_series(1,501)g),
 cf AS(INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT id,$1,'/115/admin-bulk-'||g||'.mkv',1,'present' FROM seed RETURNING *),
 ma AS(INSERT INTO media_assets(id,cloud_file_id)SELECT id,id FROM cf RETURNING *),
 mb AS(INSERT INTO media_asset_bindings(asset_id,media_id,state)SELECT id,$2,'ready' FROM ma RETURNING asset_id),
 se AS(INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT gen_random_uuid(),asset_id,'Synthetic/bulk/'||asset_id||'.strm','synthetic' FROM mb RETURNING asset_id),
 p AS(INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,state,attempts,last_error_class,last_error_reason) SELECT f.id,native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),'failed',1,'permanent','file_missing' FROM cf f JOIN se ON se.asset_id=f.id RETURNING cloud_file_id)
 SELECT count(*) FROM p`, f.storage, media).Scan(&n)
	if err != nil || n != 501 {
		t.Fatal(n, err)
	}
	repo := admin.Repository{DB: f.pool, Stills: f.store}
	page, err := repo.List(f.ctx, admin.Filter{Kind: "probe", State: "failed", PageSize: 200})
	if err != nil || page.Total != 501 || len(page.Items) != 200 {
		t.Fatal(page.Total, len(page.Items), err)
	}
	page, err = repo.List(f.ctx, admin.Filter{Kind: "probe", State: "failed", Page: 3, PageSize: 200})
	if err != nil || len(page.Items) != 101 {
		t.Fatal(len(page.Items), err)
	}
	result, err := repo.Retry(f.ctx, admin.Retry{Kind: "probe", State: "failed", Bulk: true, Confirmed: true})
	if err != nil || result.Selected != 500 || result.Queued != 500 || !result.LimitReached || result.Failed != 0 {
		t.Fatal(result, err)
	}
	if f.integer(t, `SELECT count(*) FROM media_probe_results WHERE state='failed'`) != 1 {
		t.Fatal("filter/cap bypass")
	}
	// A repeat on this filter only touches the single remaining failed entry.
	result, err = repo.Retry(f.ctx, admin.Retry{Kind: "probe", State: "failed", Bulk: true, Confirmed: true})
	if err != nil || result.Selected != 1 || result.Queued != 1 || result.LimitReached {
		t.Fatal(result, err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.probe'`) != 502 {
		t.Fatal("duplicate deterministic jobs")
	}
	if _, err = repo.Retry(f.ctx, admin.Retry{Kind: "still", Bulk: true}); err == nil {
		t.Fatal("unconfirmed batch accepted")
	}
}

func TestNativeProcessingAuditFailureRollsBackAndPreventsRetry(t *testing.T) {
	f := newStillFixture(t)
	repo := admin.Repository{DB: f.pool, Stills: f.store}
	revision := f.integer(t, `SELECT revision FROM native_server_settings`)
	for _, query := range []string{`CREATE FUNCTION reject_processing_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('native.processing.settings','native.processing.retry') THEN RAISE EXCEPTION 'synthetic audit unavailable'; END IF; RETURN NEW; END $$`, `CREATE TRIGGER reject_processing_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_processing_audit()`} {
		if _, err := f.pool.Exec(f.ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Settings(f.ctx, admin.Settings{Revision: revision}); err == nil {
		t.Fatal("settings succeeded without audit")
	}
	if f.integer(t, `SELECT revision FROM native_server_settings`) != revision || f.integer(t, `SELECT count(*) FROM native_server_settings WHERE enabled`) != 1 {
		t.Fatal("settings/audit not atomic")
	}
	if _, err := repo.Retry(f.ctx, admin.Retry{Kind: "probe", ID: f.source.asset}); err == nil {
		t.Fatal("retry proceeded without audit")
	}
	if f.text(t, `SELECT state FROM media_probe_results WHERE cloud_file_id=$1`, f.source.file) != "ready" {
		t.Fatal("retry mutated before audit")
	}
}
