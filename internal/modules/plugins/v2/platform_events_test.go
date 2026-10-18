package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/outbox"
	"strings"
	"testing"
)

func TestPlatformFactsCommitRollbackScopesTargetsAndRecursion(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	types := []string{"notification.queued", "system.operation.failed", "settings.changed", "plugin.config.changed", "messaging.inbound", "messaging.action.received"}
	meta := contracts.Snapshot{Version: 2}
	for _, typ := range types {
		meta.Descriptors = append(meta.Descriptors, contracts.Descriptor{Kind: "event", ID: typ, Handler: contracts.HandlerID("event", typ), RequiredScopes: []string{"events.subscribe." + typ}})
	}
	if e := s.Publish(ctx, l, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	facts := map[string]any{
		"notification.queued":       map[string]any{"notificationId": jobs.UUID(), "origin": "builtin"},
		"system.operation.failed":   map[string]any{"source": "jobs", "code": "terminal_failure", "count": 3, "window": "2026-10-18T00:00:00Z"},
		"settings.changed":          map[string]any{"area": "application", "revision": 2, "changed": []string{"searchCacheSeconds"}},
		"plugin.config.changed":     map[string]any{"instanceId": l.ID, "revision": 2},
		"messaging.inbound":         map[string]any{"instanceId": l.ID, "command": "hello", "actor": "opaque", "kind": "command"},
		"messaging.action.received": map[string]any{"instanceId": l.ID, "command": "hello", "actor": "opaque", "kind": "action"},
	}
	for _, commit := range []bool{false, true} {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		for typ, fact := range facts {
			v := fact.(map[string]any)
			v["botToken"] = "NEVER_BRIDGE_SECRET"
			v["stack"] = "private exception"
			v["chatId"] = "123456789"
			v["values"] = map[string]string{"masterKey": "NEVER_BRIDGE_SECRET"}
			if e = outbox.EmitVersionTx(ctx, tx, typ, l.ID, "platform-v1", v); e != nil {
				t.Fatal(e)
			}
		}
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		var n int
		if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_event_journal`).Scan(&n); e != nil || n != 0 {
			t.Fatal("uncommitted facts visible", n, e)
		}
		if commit {
			e = tx.Commit(ctx)
		} else {
			e = tx.Rollback(ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	for range 2 {
		if e := s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e := s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 6 {
		t.Fatal("all six facts", n, e)
	}
	var raw []byte
	if e := s.DB.QueryRow(ctx, `SELECT jsonb_agg(payload) FROM plugin_v2_event_journal`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"NEVER_BRIDGE_SECRET", "123456789", "stack", "botToken", "masterKey"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("leaked", secret)
		}
	}
	for _, typ := range types {
		if _, e := s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, "events.subscribe."+typ); e != nil {
			t.Fatal(e)
		}
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for typ, fact := range facts {
		if e = outbox.EmitVersionTx(ctx, tx, typ, l.ID, "platform-v2", fact); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 6 {
		t.Fatal("revoked facts", n, e)
	}
	for _, typ := range []string{"messaging.inbound", "messaging.action.received"} {
		r, _ := json.Marshal(contracts.InboundMessage{InstanceID: jobs.UUID(), Command: "hello", Actor: "opaque", Kind: "command"})
		if eventMayDeliver(typ, r, l.ID) {
			t.Fatal("cross-instance message")
		}
	}
	r, _ := json.Marshal(contracts.NotificationQueued{NotificationID: jobs.UUID(), Origin: "plugin"})
	if eventMayDeliver("notification.queued", r, l.ID) {
		t.Fatal("notification recursion")
	}
}
