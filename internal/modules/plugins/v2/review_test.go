package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReviewDiffRefreshBaselineMissingAndDataPreserved(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	d := defs[0]
	if _, e := s.hostCall(ctx, l, json.RawMessage(`{"method":"plugin.data.put","args":{"key":"retained","value":{"sentinel":true}},"expectedRevision":0}`)); e != nil {
		t.Fatal(e)
	}
	// Simulate the installed image's last approved definition, then refresh to this image.
	raw, _ := json.Marshal(d.Manifest)
	var old contracts.Manifest
	json.Unmarshal(raw, &old)
	old.Version = "0.9.0"
	old.Author = "prior maintainer"
	last := old.Permissions[len(old.Permissions)-1].Scope
	changed := old.Permissions[0].Scope
	old.Permissions = old.Permissions[:len(old.Permissions)-1]
	old.Permissions[0].Resources = map[string][]string{"roots": {"prior-root"}}
	old.Permissions = append(old.Permissions, contracts.Permission{Scope: "legacy.removed"})
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	raw, _ = json.Marshal(old)
	for _, q := range []string{`UPDATE plugin_v2_definitions SET manifest=$1,fingerprint=repeat('a',64)`, `UPDATE plugin_v2_instances SET approved_fingerprint=repeat('a',64)`, `UPDATE plugin_v2_grants SET fingerprint=repeat('a',64)`} {
		args := []any{}
		if strings.Contains(q, "$1") {
			args = append(args, raw)
		}
		if _, e = tx.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	tx.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE scope=$1`, last)
	tx.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,'legacy.removed',repeat('a',64))`, l.ID)
	if e = saveApprovalMetadata(ctx, tx, l.ID, old, true); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	before, _ := s.Inspect(ctx, l.ID, "")
	dir := filepath.Join(s.Root, l.ID, "data")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "sentinel"), []byte("data stays"), 0600)
	if e = s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.ReviewPlan(ctx, l.ID)
	if e != nil || p.ApprovedVersion != "0.9.0" || !p.BaselineKnown || !p.NeedsReview || !slices.Contains(p.Added, last) || !slices.Contains(p.Removed, "legacy.removed") || !slices.Contains(p.Changed, changed) || p.Instance.Desired != Disabled {
		t.Fatalf("%+v %v", p, e)
	}
	if p.Current.Version != d.Manifest.Version || p.Current.Author != d.Manifest.Author || len(p.Current.History) == 0 {
		t.Fatal("metadata lost", p.Current)
	}
	if e = s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	again, _ := s.ReviewPlan(ctx, l.ID)
	if again.ApprovedVersion != p.ApprovedVersion || again.Instance.Revision != p.Instance.Revision {
		t.Fatal("repeat refresh mutated review")
	}
	if e = s.refresh(ctx, nil); e != nil {
		t.Fatal(e)
	}
	p, e = s.ReviewPlan(ctx, l.ID)
	if e != nil || p.Availability != Missing || p.Current.Name != d.Manifest.Name {
		t.Fatal(p, e)
	}
	after, _ := s.Inspect(ctx, l.ID, "")
	b, _ := json.Marshal(before.Config)
	a, _ := json.Marshal(after.Config)
	if string(a) != string(b) {
		t.Fatal("configuration changed")
	}
	b, _ = json.Marshal(before.Entries)
	a, _ = json.Marshal(after.Entries)
	if len(after.Entries) != 1 || string(a) != string(b) {
		t.Fatal("KV changed or erased")
	}
	if b, e := os.ReadFile(filepath.Join(dir, "sentinel")); e != nil || string(b) != "data stays" {
		t.Fatal("data erased", e)
	}
	list, e := s.List(ctx, 0, 200)
	if e != nil || list[0].Metadata == nil || list[0].Availability != Missing {
		t.Fatal(list, e)
	}
	if e = s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	p, _ = s.ReviewPlan(ctx, l.ID)
	_, e = s.Apply(ctx, "admin", Request{Kind: Review, InstanceID: l.ID, ExpectedRevision: p.Instance.Revision - 1, OperationKey: "stale-review", Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if !errors.Is(e, ErrConflict) {
		t.Fatal("stale CAS accepted", e)
	}
	in, e := s.Apply(ctx, "admin", Request{Kind: Review, InstanceID: l.ID, ExpectedRevision: p.Instance.Revision, OperationKey: "fresh-review", Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e != nil || in.Desired != Disabled {
		t.Fatal("review auto-enabled", in, e)
	}
	p, e = s.ReviewPlan(ctx, l.ID)
	if e != nil || p.NeedsReview || len(p.Added)+len(p.Removed)+len(p.Changed) != 0 || p.ApprovedVersion != d.Manifest.Version {
		t.Fatal(p, e)
	}
}
func TestReviewLegacyBaselineUnknownCloneAndAtomicRollback(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	d := defs[0]
	s.DB.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, approvalKey(l.ID))
	s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET approved_fingerprint=repeat('b',64),reason='permission_review_required',desired_state='disabled'`)
	s.DB.Exec(ctx, `UPDATE plugin_v2_grants SET fingerprint=repeat('b',64)`)
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.ReviewPlan(ctx, l.ID)
	if e != nil || p.BaselineKnown || p.ApprovedVersion != "" || !p.NeedsReview {
		t.Fatal(p, e)
	}
	in, e := s.Apply(ctx, "admin", Request{Kind: Review, InstanceID: l.ID, ExpectedRevision: p.Instance.Revision, OperationKey: "legacy-review", Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	clone, e := s.Apply(ctx, "admin", Request{Kind: Clone, InstanceID: l.ID, ExpectedRevision: in.Revision, OperationKey: "clone-review", Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.ReviewPlan(ctx, clone.ID)
	if e != nil || !p.BaselineKnown || p.Instance.Desired != Disabled {
		t.Fatal(p, e)
	}
	counts, e := s.InstalledCounts(ctx, []string{d.Manifest.ID})
	if e != nil || counts[d.Manifest.ID] != 2 {
		t.Fatal(counts, e)
	}
	// Failure after saving the approval must roll back both the instance and summary.
	_, e = s.DB.Exec(ctx, `CREATE FUNCTION reject_plugin_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$;CREATE TRIGGER reject_plugin_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_plugin_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Apply(ctx, "admin", Request{Kind: Install, PluginID: d.Manifest.ID, OperationKey: "rollback-review", Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e == nil {
		t.Fatal("audit rejection not surfaced")
	}
	var n int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM app_settings WHERE key LIKE 'plugins.review.approved.%'`).Scan(&n)
	if n != 2 {
		t.Fatal("approval persisted after rollback", n)
	}
}
