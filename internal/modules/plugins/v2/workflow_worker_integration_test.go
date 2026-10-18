package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/testworker"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRealSameBinaryWorkflowAndBoundFakeProvider(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("mandatory native AMD64 real worker gate")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	s := fixture(t)
	if _, e := s.DB.Exec(context.Background(), `CREATE TABLE ops_mode (LIKE public.ops_mode INCLUDING ALL); INSERT INTO ops_mode(singleton,paused)VALUES(true,false)`); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	in, e := s.Apply(ctx, "admin", Request{Kind: Install, OperationKey: jobs.UUID(), PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), Config: json.RawMessage(`{"scheduled":false}`), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "admin", Request{Kind: Enable, OperationKey: jobs.UUID(), InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	live, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- (Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: slog.Default(), MaxWorkers: 1}).Run(live)
	}()
	t.Cleanup(func() {
		stop()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker reaper timeout")
		}
	})
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, err := s.UI(c, in.ID, "form", ""); return err })
	r := WorkflowRunner{s}
	w := contracts.Workflow{ID: "real-workflow", Steps: []contracts.WorkflowStep{{ID: "one", InstanceID: in.ID, Action: "hello", Input: contracts.ActionContext{Data: map[string]json.RawMessage{}}}, {ID: "two", DependsOn: []string{"one"}, InstanceID: in.ID, Action: "f4-check", Input: contracts.ActionContext{Data: map[string]json.RawMessage{}}}}}
	def, e := r.Save(ctx, "admin", w, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Approve(ctx, "admin", w.ID, def.Fingerprint); e != nil {
		t.Fatal(e)
	}
	run, e := r.Start(ctx, "admin", w.ID, "real-workflow-key")
	if e != nil {
		t.Fatal(e)
	}
	repo := jobs.Repository{DB: s.DB}
	j, e := repo.Claim(ctx, "workflow-test", time.Minute)
	if e != nil || j == nil || j.Kind != "plugin.v2.workflow" {
		t.Fatal(j, e)
	}
	if e = r.Process(ctx, *j); e != nil {
		t.Fatal(e)
	}
	if e = repo.Finish(ctx, *j, nil); e != nil {
		t.Fatal(e)
	}
	status, e := r.Status(ctx, w.ID, run.ID)
	if e != nil || status.State != "succeeded" {
		t.Fatal(status, e)
	}
	if _, e = s.DB.Exec(ctx, `CREATE TABLE administrator_accounts (LIKE public.administrator_accounts INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	account := jobs.UUID()
	if _, e = s.DB.Exec(ctx, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES($1,'fixture','not-a-login-hash')`, account); e != nil {
		t.Fatal(e)
	}
	broker := Broker{s}
	if e = broker.Bind(ctx, "admin", in.ID, "fake", "bound", account, d.Fingerprint); e != nil {
		t.Fatal(e)
	}
	q, e := broker.Begin(ctx, in.ID, "fake", "https://hello.invalid")
	if e != nil {
		t.Fatal(e)
	}
	identity, e := broker.Verify(ctx, in.ID, "fake", "https://hello.invalid", q.State, q.Nonce, "bound")
	if e != nil || identity.AccountID != account {
		t.Fatal(identity, e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(10 * time.Second)
	for {
		var received bool
		if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs WHERE instance_id=$1 AND handler=$2 AND state='succeeded')`, in.ID, contracts.HandlerID("event", "workflow.started")).Scan(&received); e != nil {
			t.Fatal(e)
		}
		if received {
			break
		}
		if time.Now().After(until) {
			t.Fatal("real worker did not receive committed workflow.started")
		}
		time.Sleep(30 * time.Millisecond)
	}
	var rev int64
	if e = s.DB.QueryRow(ctx, `SELECT revision FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&rev); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "admin", Request{Kind: Disable, InstanceID: in.ID, ExpectedRevision: rev, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	if e = broker.Validate(ctx, &identity); e == nil {
		t.Fatal("disabled provider identity survived")
	}
}
