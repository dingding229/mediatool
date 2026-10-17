package v2

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func activeFixture(t *testing.T) (Store, Lease) {
	t.Helper()
	s := fixture(t)
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	in, e := s.Apply(context.Background(), "admin", Request{Kind: Install, OperationKey: "install-test", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d)})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Apply(context.Background(), "admin", Request{Kind: Enable, OperationKey: "enable-test", InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Claim(context.Background(), jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	return s, l
}
func TestHostConcurrentReadsRenewRevocationAndWriteFence(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT id FROM plugin_v2_instances WHERE id=$1 FOR KEY SHARE`, l.ID); e != nil {
		t.Fatal(e)
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	raw := json.RawMessage(`{"method":"plugin.state.get","args":{}}`)
	if _, e = s.hostCall(bounded, l, raw); e != nil {
		t.Fatal("concurrent reader blocked", e)
	}
	if e = s.Renew(bounded, l, true); e != nil {
		t.Fatal("lease renewal blocked", e)
	}
	// Stage a write before revocation. Its final live fence must roll it back.
	write, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, write)
	if _, e = write.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'plugin.notice',$2,'{}')`, jobs.UUID(), l.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(bounded, `UPDATE plugin_v2_instances SET grant_revision=grant_revision+1 WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.hostCall(ctx, l, raw); e == nil {
		t.Fatal("revoked call allowed")
	}
	if e = fenceHost(ctx, write, l, "plugin.data.write"); e == nil {
		t.Fatal("stale write fence allowed")
	}
	if e = write.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&n); e != nil || n != 0 {
		t.Fatal("side effect escaped rollback", n, e)
	}
}
