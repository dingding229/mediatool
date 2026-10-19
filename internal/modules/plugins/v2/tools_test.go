package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/platform/pluginstate"
	"testing"
	"time"
)

func toolFixture(t *testing.T) (Store, Lease, contracts.Descriptor) {
	t.Helper()
	s, l := activeFixture(t)
	ctx := context.Background()
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	d := contracts.Descriptor{Kind: "tool", ID: "hello.echo", Name: "hello.echo", Description: "fixture echo", Handler: contracts.HandlerID("tool", "hello.echo"), RequiredScopes: []string{"agent.tools"}, EffectKind: "read", InputSchema: json.RawMessage(`{"type":"object","required":["message"],"properties":{"message":{"type":"string","maxLength":200}},"additionalProperties":false}`)}
	if e := s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); e != nil {
		t.Fatal(e)
	}
	return s, l, d
}
func TestToolSchemaAuthorizationAuditAndIdempotentHistory(t *testing.T) {
	s, l, d := toolFixture(t)
	ctx := context.Background()
	if _, e := s.ToolCall(ctx, "admin", l.ID, d.ID, "invalid-params", json.RawMessage(`{"message":3}`), false); e == nil {
		t.Fatal("schema bypass")
	}
	// Complete the real durable mailbox row from a fake worker, without live media.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := 0; n < 100; n++ {
			var id string
			if s.DB.QueryRow(ctx, `SELECT id::text FROM plugin_v2_runs WHERE kind='tool'`).Scan(&id) == nil {
				cipher, e := s.seal("call-result", id, json.RawMessage(`{"message":"ok"}`))
				if e != nil {
					t.Error(e)
					return
				}
				_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='succeeded',result_ciphertext=$2,finished_at=now() WHERE id=$1`, id, cipher)
				if e != nil {
					t.Error(e)
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	out, e := s.ToolCall(ctx, "admin", l.ID, d.ID, "valid-tool-key", json.RawMessage(`{"message":"ok"}`), false)
	<-done
	if e != nil || string(out) != `{"message":"ok"}` {
		t.Fatal(string(out), e)
	}
	if _, e = s.ToolCall(ctx, "admin", l.ID, d.ID, "valid-tool-key", json.RawMessage(`{"message":"ok"}`), false); e != nil {
		t.Fatal(e)
	}
	var n int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='tool'`).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate run", n)
	}
	s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='agent.tools'`, l.ID)
	if _, e = s.ToolCall(ctx, "admin", l.ID, d.ID, "revoked-tool-key", json.RawMessage(`{"message":"ok"}`), false); e == nil {
		t.Fatal("revoked accepted")
	}
}
func TestToolEffectCannotUnderstateAndScopeCeiling(t *testing.T) {
	if business.ValidToolEffect("read", []string{"settings.write"}) || business.ValidToolEffect("write", []string{"system.exec"}) {
		t.Fatal("danger understated")
	}
	s, l, d := toolFixture(t)
	gate := &hostGate{}
	gate.enterFor(d.RequiredScopes, true)
	_, e := s.scopedHost(l, gate)(context.Background(), json.RawMessage(`{"method":"plugin.data.put","args":{"key":"forbidden","value":true},"expectedRevision":0}`))
	if e == nil {
		t.Fatal("tool escaped declared scopes")
	}
}
func TestWorkflowReadViewsAndAuthBindingAttribution(t *testing.T) {
	s, l, _ := toolFixture(t)
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	d := WorkflowDefinition{DAG: contracts.Workflow{ID: "schema-form", Steps: []contracts.WorkflowStep{}}, Revision: 1, Fingerprint: "hash", Approved: "hash"}
	if e = pluginstate.Lock(ctx, tx, "plugins.workflow.schema-form", &WorkflowDefinition{}); e != nil {
		t.Fatal(e)
	}
	if e = pluginstate.Put(ctx, tx, "plugins.workflow.schema-form", d); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	list, e := (WorkflowRunner{s}).List(ctx, 1)
	if e != nil || len(list) != 1 || list[0].ID != "schema-form" {
		t.Fatal(list, e)
	}
	if _, e = s.DB.Exec(ctx, `CREATE TABLE administrator_accounts (LIKE public.administrator_accounts INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	status, e := (Broker{s}).Status(ctx, l.ID)
	if e != nil || len(status) != 1 || status[0].Bound != 0 || status[0].Error == "" {
		t.Fatal(status, e)
	}
}
