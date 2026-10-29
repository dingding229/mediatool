package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTaskControlHTTPPermissionCASAndUnknownReceipt(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	repo := jobs.Repository{DB: s.DB}
	j, err := repo.Create(ctx, "system.check", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg()
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	h := New(c, Dependencies{DB: s.DB, AuthSessions: testSessionReader{}})
	call := func(method, path, body string, cookie, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/tasks/jobs/" + j.ID
	if w := call("GET", path, "", false, false); w.Code != 401 {
		t.Fatal("anonymous control", w.Code)
	}
	if w := call("POST", path+"/cancel", `{"confirm":true,"revision":1}`, true, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := call("POST", path+"/cancel", `{"confirm":false,"revision":1}`, true, true); w.Code != 400 {
		t.Fatal("confirmation bypass")
	}
	w := call("POST", path+"/cancel", `{"confirm":true,"revision":1}`, true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Data jobs.ControlResult `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Data.State != "cancelled" {
		t.Fatal(err, w.Body.String())
	}
	w = call("POST", path+"/retry", `{"confirm":true,"revision":2}`, true, true)
	if w.Code != 202 || !strings.Contains(w.Body.String(), `"queued":true`) {
		t.Fatal("queued implies completed?", w.Code, w.Body.String())
	}
	if w = call("POST", path+"/retry", `{"confirm":true,"revision":2}`, true, true); w.Code != 409 {
		t.Fatal("same revision duplicate accepted")
	}
	// A task without a safe adapter must remain unknown, not become runnable.
	unknown, err := repo.Create(ctx, "fixture.unsafe", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE jobs SET state='unknown' WHERE id=$1`, unknown.ID); err != nil {
		t.Fatal(err)
	}
	w = call("POST", "/api/v1/tasks/jobs/"+unknown.ID+"/retry", `{"confirm":true,"revision":1}`, true, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unknown"`) || !strings.Contains(w.Body.String(), `"queued":false`) {
		t.Fatal("unsafe retry queued", w.Code, w.Body.String())
	}
}

func TestTaskControlNoCredentialsAndMissingAdaptersRemainUnknown(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	api := &API{Deps: Dependencies{DB: s.DB}}
	repo := jobs.Repository{DB: s.DB}
	for _, kind := range []string{"download.add", "transfer.upload", "download.manual.add", "ops.restore"} {
		j, err := repo.Create(ctx, kind, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(ctx, `UPDATE jobs SET state='unknown' WHERE id=$1`, j.ID); err != nil {
			t.Fatal(err)
		}
		out, err := repo.Retry(ctx, j.ID, 1, api.verifyTask)
		if err != nil || out.State != "unknown" || out.Queued {
			t.Fatal("missing adapter queued", kind, out, err)
		}
	}
}

func TestDownloadTaskControlDoesNotRouteToRetiredEmbyJobs(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	a := API{Deps: Dependencies{DB: s.DB}}
	for _, state := range []string{"downloading", "strm_ready", "refresh_requested", "imported"} {
		task, active := jobs.UUID(), jobs.UUID()
		if _, err := s.DB.Exec(ctx, `INSERT INTO download_tasks(id,download_state)VALUES($1,$2)`, task, state); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(ctx, `INSERT INTO jobs(id,kind,state,idempotency_key)VALUES($1,'download.add','succeeded',$2)`, active, "download.add:"+task); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"emby.refresh", "emby.confirm"} {
			if _, err := s.DB.Exec(ctx, `INSERT INTO jobs(id,kind,state,idempotency_key)VALUES($1,$2,'succeeded',$3)`, jobs.UUID(), kind, kind+":"+task); err != nil {
				t.Fatal(err)
			}
		}
		got, err := a.taskJob(ctx, "downloads", task)
		if err != nil || got != active {
			t.Fatal("download mapped to retired job", state, got, err)
		}
	}
}
