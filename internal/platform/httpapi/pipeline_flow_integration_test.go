package httpapi

import (
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPipelineRetryPreservesUnknownAndMissingJobs(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	api := API{Deps: Dependencies{DB: s.DB, Integrations: &integrations.Manager{}}}
	task, job := jobs.UUID(), jobs.UUID()
	if _, e := s.DB.Exec(ctx, `INSERT INTO download_tasks(id)VALUES($1)`, task); e != nil {
		t.Fatal(e)
	}
	call := func(kind, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/", nil)
		r.SetPathValue("kind", kind)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		api.retryPipeline(w, r)
		return w
	}
	if w := call("downloads", jobs.UUID()); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("downloads", task); w.Code != 409 || !strings.Contains(w.Body.String(), `"reason":"job_missing"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO jobs(id,kind,state,idempotency_key)VALUES($1,'download.add','failed',$2)`, job, "download.add:"+task); e != nil {
		t.Fatal(e)
	}
	if w := call("downloads", strings.ToUpper(task)); w.Code != 200 || !strings.Contains(w.Body.String(), `"reason":"retry_queued"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("downloads", task); w.Code != 409 {
		t.Fatal("duplicate retry", w.Code, w.Body.String())
	}
	transfer := jobs.UUID()
	if _, e := s.DB.Exec(ctx, `INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,state,upload_id)VALUES($1,$2,$3,'Synthetic/unknown.mkv','synthetic','uncertain','known-session')`, transfer, jobs.UUID(), jobs.UUID()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO jobs(id,kind,state,idempotency_key)VALUES($1,'transfer.upload','failed',$2)`, jobs.UUID(), "transfer:"+transfer); e != nil {
		t.Fatal(e)
	}
	if w := call("transfers", transfer); w.Code != 409 || !strings.Contains(w.Body.String(), `"nextAction":"reconcile_upload"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	var state string
	if e := s.DB.QueryRow(ctx, `SELECT state FROM jobs WHERE idempotency_key=$1`, "transfer:"+transfer).Scan(&state); e != nil || state != "failed" {
		t.Fatal("unknown replayed", state, e)
	}
}
