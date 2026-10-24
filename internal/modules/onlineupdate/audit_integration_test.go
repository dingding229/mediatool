package onlineupdate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalAuditReconcilesEveryPreDSNFailure(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("media_tool_test required")
	}
	db, _ := helperTestDB(t)
	for _, name := range []string{"missing_plan", "identity_mismatch", "missing_dsn"} {
		t.Run(name, func(t *testing.T) {
			d := testDocker()
			root := t.TempDir()
			id, e := newID()
			if e != nil {
				t.Fatal(e)
			}
			j := Job{ID: id, Actor: "fixture-admin", Phase: "queued", FromImage: fmtString(d.self.Config["Image"]), CreatedAt: time.Now()}
			if e = save(root, j); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _, _ = db.Exec(context.Background(), `DELETE FROM audit_logs WHERE details->>'id'=$1`, id) })
			original := d.self
			raw, _ := json.Marshal(newPlan(original, d.target, id))
			if name != "missing_plan" {
				if e = os.WriteFile(filepath.Join(root, id+".plan"), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if name == "identity_mismatch" {
				original.Config["User"] = "0:0"
			}
			body, _ := json.Marshal(original)
			d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, apiVersion+"/containers/") {
					t.Fatal("pre-admission mutation", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			if e = runUpdater(t.Context(), root, id, d); e == nil {
				t.Fatal("unsafe input accepted")
			}
			saved, e := load(root, id)
			if e != nil || saved.Phase != "failed" {
				t.Fatal(e, saved.Phase)
			}
			if name == "missing_dsn" {
				newer, _ := newID()
				if e = save(root, Job{ID: newer, Phase: "queued", CreatedAt: time.Now().Add(time.Minute)}); e != nil {
					t.Fatal(e)
				}
			}
			s := &Service{Root: root, DB: db}
			// A helper that has not exited still holds the execution lock: app waits.
			unlock, e := lock(root, "execution")
			if e != nil {
				t.Fatal(e)
			}
			if e = s.reconcileAudits(t.Context()); e != nil {
				t.Fatal(e)
			}
			var premature int
			if e = db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='online_update.failed' AND details->>'id'=$1`, id).Scan(&premature); e != nil || premature != 0 {
				t.Fatal("reconciled before helper exit", e, premature)
			}
			unlock()
			for range 2 {
				if e = s.reconcileAudits(t.Context()); e != nil {
					t.Fatal(e)
				}
			}
			var n int
			var source string
			if e = db.QueryRow(t.Context(), `SELECT count(*),min(details->>'auditSource') FROM audit_logs WHERE action='online_update.failed' AND details->>'id'=$1`, id).Scan(&n, &source); e != nil || n != 1 || source != "reconciliation" {
				t.Fatal(e, n, source)
			}
		})
	}
}
