package migrations_test

import (
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/secrets"
	"reflect"
	"strings"
	"testing"
)

func TestPluginStateMigrationUpDownUpPreservesNonPluginData(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 63); e != nil {
		t.Fatal(e)
	}
	before := pluginUnrelatedSnapshot(t, f)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 64); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(before, pluginUnrelatedSnapshot(t, f)) {
			t.Fatal("non-plugin changed")
		}
		if _, e := f.provider.DownTo(f.ctx, 63); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.provider.UpTo(f.ctx, 64); e != nil {
		t.Fatal(e)
	}
}

func TestPluginStateSnapshotRekeyKeepsRestorableAAD(t *testing.T) {
	f, s := pluginStore(t)
	if _, e := f.provider.UpTo(f.ctx, 74); e != nil {
		t.Fatal(e)
	}
	f.version(t, 74)
	t.Log("schema71 constrained snapshot/rekey/restore round trip")
	q := installRequest(t)
	in, e := s.Apply(f.ctx, "admin", q)
	if e != nil {
		t.Fatal(e)
	}
	d, _ := v2.Catalog()
	snap, e := s.StateOperation(f.ctx, "admin", in.ID, v2.StateRequest{Kind: "create", OperationKey: "state-rekey-create", ExpectedRevision: in.Revision, Fingerprint: d[0].Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	next, e := secrets.New(strings.Repeat("66", 32))
	if e != nil {
		t.Fatal(e)
	}
	counts, e := secrets.Rekey(f.ctx, s.DB, s.Vault, next, false)
	if e != nil || counts["plugin_v2_state_values"] != 1 {
		t.Fatal(counts, e)
	}
	s.Vault = next
	info, e := s.Inspect(f.ctx, in.ID, "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.StateOperation(f.ctx, "admin", in.ID, v2.StateRequest{Kind: "restore", SnapshotID: snap.ID, OperationKey: "state-rekey-restore", ExpectedRevision: info.Instance.Revision, Fingerprint: d[0].Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
}
