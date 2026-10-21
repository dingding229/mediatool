package v2

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestContentFingerprintIndependentOfReleaseAndCanonicalJSON(t *testing.T) {
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	var inventory struct {
		ContentHashes map[string]string `json:"contentHashes"`
	}
	if e = json.Unmarshal(libraryInventory, &inventory); e != nil {
		t.Fatal(e)
	}
	for _, d := range defs {
		raw, e := canonicalManifest(d.Manifest)
		if e != nil {
			t.Fatal(e)
		}
		expected := "content-v1:" + digest(append(raw, []byte(inventory.ContentHashes[d.Manifest.ID])...))
		if d.Fingerprint != expected {
			t.Fatal(d.Manifest.ID)
		}
		if !strings.HasPrefix(expected, "content-v1:") {
			t.Fatal(expected)
		}
		changed := d.Manifest
		changed.Description += "changed"
		v, _ := canonicalManifest(changed)
		if digest(v) == digest(raw) {
			t.Fatal("description excluded")
		}
		if digest(append(raw, []byte("different-handler-or-esm-hash")...)) == strings.TrimPrefix(d.Fingerprint, "content-v1:") {
			t.Fatal("behavior excluded")
		}
	}
}
func TestLegacyContentEquivalentRebindWithoutRevisionOrStop(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	d, e := definition(l.PluginID)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(d.Manifest)
	old := digest(append(raw, []byte(BuildID())...))
	if legacyContent(old) != d.Fingerprint {
		t.Fatal("missing equivalent locked source evidence")
	}
	for _, sql := range []string{`UPDATE plugin_v2_definitions SET fingerprint=$1`, `UPDATE plugin_v2_instances SET approved_fingerprint=$1`, `UPDATE plugin_v2_grants SET fingerprint=$1`} {
		if _, e = s.DB.Exec(ctx, sql, old); e != nil {
			t.Fatal(e)
		}
	}
	snapshot := func() string {
		t.Helper()
		var v string
		if e = s.DB.QueryRow(ctx, `SELECT row(revision,grant_revision,desired_state,runtime_state,generation,reason)::text FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := snapshot()
	for range 2 {
		if e = s.Refresh(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if snapshot() != before {
		t.Fatal("migration changed state/revision")
	}
	var fp string
	var rev int
	if e = s.DB.QueryRow(ctx, `SELECT fingerprint,revision FROM plugin_v2_definitions`).Scan(&fp, &rev); e != nil || fp != d.Fingerprint || rev != 1 {
		t.Fatal(fp, rev, e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&fp); e != nil || fp != d.Fingerprint {
		t.Fatal(fp, e)
	}
}
func TestDisabledAndReviewAccessGuidance(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	for _, state := range []struct{ desired, reason, want string }{{"disabled", "", "已停用"}, {"disabled", "permission_review_required", "权限待确认"}} {
		if _, e := s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state=$2,reason=$3 WHERE id=$1`, l.ID, state.desired, state.reason); e != nil {
			t.Fatal(e)
		}
		_, e := s.Snapshot(ctx, l.ID)
		if e == nil {
			t.Fatal("state unexpectedly available")
		}
		v := accessError(ctx, s.DB, l.ID)
		raw, _ := json.Marshal(v)
		if !strings.Contains(string(raw), state.want) {
			t.Fatal(string(raw))
		}
	}
}

func TestContentHandlerOnlyChangeStillAdvancesRevision(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	for i := range defs {
		if defs[i].Manifest.ID == l.PluginID {
			defs[i].Fingerprint = "content-v1:" + digest([]byte("changed-handler-or-esm"))
		}
	}
	if e = s.refresh(ctx, defs); e != nil {
		t.Fatal(e)
	}
	var rev int
	var desired, reason string
	if e = s.DB.QueryRow(ctx, `SELECT d.revision,i.desired_state,i.reason FROM plugin_v2_definitions d JOIN plugin_v2_instances i ON i.plugin_id=d.id WHERE i.id=$1`, l.ID).Scan(&rev, &desired, &reason); e != nil || rev != 2 || desired != "enabled" || reason != "" {
		t.Fatal(rev, desired, reason, e)
	}
}

func TestLegacySameManifestWithoutContentEvidenceIsNotSilentlyAdopted(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	old := digest([]byte("old-different-handler-same-manifest"))
	for _, sql := range []string{`UPDATE plugin_v2_definitions SET fingerprint=$1`, `UPDATE plugin_v2_instances SET approved_fingerprint=$1`, `UPDATE plugin_v2_grants SET fingerprint=$1`} {
		if _, e := s.DB.Exec(ctx, sql, old); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	var revision int
	var desired, reason string
	if e := s.DB.QueryRow(ctx, `SELECT d.revision,i.desired_state,i.reason FROM plugin_v2_definitions d JOIN plugin_v2_instances i ON i.plugin_id=d.id WHERE i.id=$1`, l.ID).Scan(&revision, &desired, &reason); e != nil || revision != 2 || desired != "enabled" || reason != "" {
		t.Fatal("unproven legacy must refresh without false permission expansion", revision, desired, reason, e)
	}
}
