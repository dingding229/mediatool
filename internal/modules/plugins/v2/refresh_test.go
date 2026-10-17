package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"sync"
	"testing"
)

func TestRefreshIdempotentCASReviewMissingAndRetainedData(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	d := defs[0]
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.Refresh(ctx); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var rev int64
	if e := s.DB.QueryRow(ctx, `SELECT revision FROM plugin_v2_definitions`).Scan(&rev); e != nil || rev != 1 {
		t.Fatal("non-idempotent", rev, e)
	}
	d.Manifest.Version = "1.0.1"
	d.Manifest.History["1.0.1"] = "fixture"
	d.Fingerprint = digest([]byte("new-pin"))
	if e := s.refresh(ctx, []Definition{d}); e != nil {
		t.Fatal(e)
	}
	var approved, desired string
	var grants int64
	if e := s.DB.QueryRow(ctx, `SELECT approved_fingerprint,desired_state,grant_revision FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&approved, &desired, &grants); e != nil || approved != d.Fingerprint || desired != "enabled" {
		t.Fatal(approved, desired, e)
	}
	if _, e := s.hostCall(ctx, l, json.RawMessage(`{"method":"plugin.state.get","args":{}}`)); e == nil {
		t.Fatal("old generation accepted")
	}
	d.Manifest.Permissions = append(d.Manifest.Permissions, contracts.Permission{Scope: "system.exec"})
	d.Fingerprint = digest([]byte("expanded-pin"))
	if e := s.refresh(ctx, []Definition{d}); e != nil {
		t.Fatal(e)
	}
	var why string
	if e := s.DB.QueryRow(ctx, `SELECT desired_state,reason,grant_revision FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&desired, &why, &grants); e != nil || desired != "disabled" || why != string(PermissionReview) {
		t.Fatal(desired, why, e)
	}
	if e := s.Finish(ctx, l, NoReason); e != nil {
		t.Fatal(e)
	}
	if e := s.DB.QueryRow(ctx, `SELECT reason FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&why); e != nil || why != string(PermissionReview) {
		t.Fatal("worker finish erased required review", why, e)
	}
	before := grants
	if e := s.refresh(ctx, []Definition{d}); e != nil {
		t.Fatal(e)
	}
	if e := s.DB.QueryRow(ctx, `SELECT grant_revision FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&grants); e != nil || grants != before {
		t.Fatal("review reset by repeated sync", e)
	}
	if e := s.refresh(ctx, nil); e != nil {
		t.Fatal(e)
	}
	var n int
	var availability string
	if e := s.DB.QueryRow(ctx, `SELECT availability FROM plugin_v2_definitions`).Scan(&availability); e != nil || availability != "missing" {
		t.Fatal(e, availability)
	}
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_configs WHERE id=$1`, l.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal("data erased", e)
	}
	// Return to the actual compiled pin, re-confirmation remains explicit.
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	current, e := s.List(ctx, 0, 200)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Apply(ctx, "admin", Request{Kind: Review, OperationKey: "review-current", InstanceID: l.ID, ExpectedRevision: current[0].Revision, Fingerprint: defs[0].Fingerprint, Confirm: true, Scopes: scopes(defs[0])})
	if e != nil {
		t.Fatal(e)
	}
	if out.Desired != Disabled || out.Reason != NoReason {
		t.Fatal(out)
	}
}

func TestPinnedInventoryAndSDKSelfCheck(t *testing.T) {
	if e := SelfCheck(); e != nil {
		t.Fatal(e)
	}
	raw := Resources()
	raw[0] = 'x'
	if e := SelfCheck(); e != nil {
		t.Fatal("mutable shared resource inventory", e)
	}
}

func TestAdminInspectionAndRetainedRemovalAreBounded(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	out, e := s.Inspect(ctx, l.ID, "")
	if e != nil || out.Instance.ID != l.ID || out.Config.Revision != 1 || len(out.Entries) != 0 {
		t.Fatal(out, e)
	}
	if _, e = s.Inspect(ctx, "../other", ""); e == nil {
		t.Fatal("path id accepted")
	}
	current, e := s.List(ctx, 0, 50)
	if e != nil {
		t.Fatal(e)
	}
	req := Request{Kind: Remove, OperationKey: "remove-retain", InstanceID: l.ID, ExpectedRevision: current[0].Revision, Fingerprint: l.Fingerprint, Confirm: true}
	removed, e := s.Apply(ctx, "admin", req)
	if e != nil || removed.Desired != Absent {
		t.Fatal(removed, e)
	}
	replay, e := s.Apply(ctx, "admin", req)
	if e != nil || replay != removed {
		t.Fatal("removal replay not idempotent", e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_configs WHERE id=$1`, l.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal("removed private data", n, e)
	}
	if _, e = s.Inspect(ctx, l.ID, ""); e == nil {
		t.Fatal("absent instance inspectable")
	}
}

func TestDisableDoesNotEraseRequiredConsent(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state='disabled',runtime_state='stopping',reason='permission_review_required',approved_fingerprint=repeat('0',64) WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	items, e := s.List(ctx, 0, 50)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Apply(ctx, "admin", Request{Kind: Disable, OperationKey: "disable-review", InstanceID: l.ID, ExpectedRevision: items[0].Revision, Fingerprint: l.Fingerprint, Confirm: true})
	if e != nil || out.Reason != PermissionReview {
		t.Fatal("disable erased required consent", out, e)
	}
}
