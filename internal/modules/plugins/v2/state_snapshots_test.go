package v2

import (
	"context"
	"encoding/json"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/secrets"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stateFixture(t *testing.T) (Store, Instance, Definition) {
	t.Helper()
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	d := defs[0]
	if _, e := s.hostCall(ctx, l, json.RawMessage(`{"method":"plugin.data.put","args":{"key":"retained","value":{"private":"snapshot-sentinel"}},"expectedRevision":0}`)); e != nil {
		t.Fatal(e)
	}
	list, _ := s.List(ctx, 0, 200)
	in, e := s.Apply(ctx, "admin", Request{Kind: Disable, InstanceID: l.ID, OperationKey: "state-disable", ExpectedRevision: list[0].Revision, Fingerprint: d.Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, l, NoReason); e != nil {
		t.Fatal(e)
	}
	if e = s.EnsurePaths(in.ID); e != nil {
		t.Fatal(e)
	}
	return s, in, d
}
func stateRequest(t *testing.T, s Store, id string, d Definition, kind, key, snapshot string) StateRequest {
	t.Helper()
	in, e := s.Inspect(context.Background(), id, "")
	if e != nil {
		t.Fatal(e)
	}
	return StateRequest{Kind: kind, OperationKey: key, SnapshotID: snapshot, ExpectedRevision: in.Instance.Revision, Fingerprint: d.Fingerprint, Confirm: true}
}
func TestStateSnapshotRoundTripCipherCASAndRetention(t *testing.T) {
	s, in, d := stateFixture(t)
	ctx := context.Background()
	data := filepath.Join(s.Root, in.ID, "data", "owned.json")
	os.WriteFile(data, []byte(`{"private":"before"}`), 0600)
	q := stateRequest(t, s, in.ID, d, "create", "state-create", "")
	snap, e := s.StateOperation(ctx, "admin", in.ID, q)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.StateOperation(ctx, "admin", in.ID, q)
	if e != nil || again.ID != snap.ID {
		t.Fatal("receipt", again, e)
	}
	var raw []byte
	if e = s.DB.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_state_values WHERE snapshot_id=$1 AND kind='kv'`, snap.ID).Scan(&raw); e != nil || strings.Contains(string(raw), "snapshot-sentinel") {
		t.Fatal("plaintext snapshot", e)
	}
	// Stored values use the normal Vault/AAD contract and survive a complete rekey.
	next, _ := secrets.New(strings.Repeat("55", 32))
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := tx.Query(ctx, `SELECT id::text,ciphertext FROM plugin_v2_state_values`)
	if e != nil {
		t.Fatal(e)
	}
	type v struct {
		id string
		b  []byte
	}
	vs := []v{}
	for rows.Next() {
		var x v
		rows.Scan(&x.id, &x.b)
		vs = append(vs, x)
	}
	rows.Close()
	for _, x := range vs {
		p, e := s.Vault.Open("plugin:v2:state-value:"+x.id, x.b)
		if e != nil {
			t.Fatal(e)
		}
		c, _ := next.Seal("plugin:v2:state-value:"+x.id, p)
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_values SET ciphertext=$2 WHERE id=$1`, x.id, c); e != nil {
			t.Fatal(e)
		}
	}
	tx.Rollback(ctx) // main rekey integration verifies all columns; this proves AAD compatibility.
	os.WriteFile(data, []byte(`{"private":"after"}`), 0600)
	before, _ := s.Inspect(ctx, in.ID, "")
	_, e = s.Apply(ctx, "admin", Request{Kind: Configure, InstanceID: in.ID, OperationKey: "state-change", ExpectedRevision: before.Instance.Revision, Fingerprint: d.Fingerprint, Confirm: true, Config: json.RawMessage(`{"changed":true}`)})
	if e != nil {
		t.Fatal(e)
	}
	stale := stateRequest(t, s, in.ID, d, "restore", "state-stale", snap.ID)
	stale.ExpectedRevision--
	if _, e = s.StateOperation(ctx, "admin", in.ID, stale); e == nil {
		t.Fatal("stale CAS accepted")
	}
	restore := stateRequest(t, s, in.ID, d, "restore", "state-restore", snap.ID)
	if _, e = s.StateOperation(ctx, "admin", in.ID, restore); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(data)
	if string(b) != `{"private":"before"}` {
		t.Fatal("file not restored")
	}
	got, _ := s.Inspect(ctx, in.ID, "")
	if got.Instance.Desired != Disabled || strings.Contains(string(got.Config.Value), "changed") || len(got.Entries) != 1 {
		t.Fatal("state not restored", got)
	}
	if _, e = s.StateOperation(ctx, "admin", in.ID, restore); e != nil {
		t.Fatal("restore receipt replay", e)
	}
	if _, e = s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "delete", "state-delete", snap.ID)); e != nil {
		t.Fatal(e)
	}
	items, e := s.StateSnapshots(ctx, in.ID)
	if e != nil || len(items) != 0 {
		t.Fatal(items, e)
	}
	if _, e = os.Stat(filepath.Join(s.Root, in.ID, filepath.FromSlash(snap.DataPath))); !os.IsNotExist(e) {
		t.Fatal("deleted files retained", e)
	}
}
func TestStateRestoreFailedCommitRollsBackExchange(t *testing.T) {
	s, in, d := stateFixture(t)
	ctx := context.Background()
	path := filepath.Join(s.Root, in.ID, "data", "owned.txt")
	os.WriteFile(path, []byte("snapshot"), 0600)
	snap, e := s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "fault-create", ""))
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("current"), 0600)
	_, e = s.DB.Exec(ctx, `CREATE FUNCTION reject_state_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='plugins.state.restore.completed' THEN RAISE EXCEPTION 'fixture crash before commit'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_state_commit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_state_commit()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "restore", "fault-restore", snap.ID)); e == nil {
		t.Fatal("failed commit reported success")
	}
	if e = s.RecoverState(ctx); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "current" {
		t.Fatal("uncommitted exchange not undone", string(b))
	}
	var busy bool
	s.DB.QueryRow(ctx, `SELECT data_operation IS NOT NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&busy)
	if busy {
		t.Fatal("fence not released")
	}
}
func TestStateSnapshotRejectsLinksExecutablesIncompatibleAndOtherInstance(t *testing.T) {
	s, in, d := stateFixture(t)
	ctx := context.Background()
	dir := filepath.Join(s.Root, in.ID, "data")
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("secret"), 0600)
	for _, kind := range []string{"symlink", "hardlink", "executable", "binary"} {
		p := filepath.Join(dir, "unsafe")
		switch kind {
		case "symlink":
			os.Symlink(outside, p)
		case "hardlink":
			os.Link(outside, p)
		case "executable":
			os.WriteFile(p, []byte("code"), 0700)
		case "binary":
			os.WriteFile(p, []byte("\x7fELFcode"), 0600)
		}
		if _, e := s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "reject-"+kind, "")); e == nil {
			t.Fatal(kind, "accepted")
		}
		os.Remove(p)
	}
	snap, e := s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "valid-create", ""))
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Exec(ctx, `UPDATE plugin_v2_state_snapshots SET data_schema=data_schema+1 WHERE id=$1`, snap.ID)
	if _, e = s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "restore", "reject-schema", snap.ID)); e == nil {
		t.Fatal("schema accepted")
	}
	s.DB.Exec(ctx, `UPDATE plugin_v2_state_snapshots SET data_schema=$2 WHERE id=$1`, snap.ID, d.Manifest.DataSchemaVersion)
	other, e := s.Apply(ctx, "admin", Request{Kind: Install, OperationKey: "state-other", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StateOperation(ctx, "admin", other.ID, stateRequest(t, s, other.ID, d, "restore", "reject-other", snap.ID)); e == nil {
		t.Fatal("cross instance restored")
	}
	for n := 0; n < 4; n++ {
		q := stateRequest(t, s, in.ID, d, "create", "retention-"+string(rune('a'+n)), "")
		if _, e = s.StateOperation(ctx, "admin", in.ID, q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "reject-retention", "")); e == nil {
		t.Fatal("count quota accepted")
	}
	var busy bool
	s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET data_operation=gen_random_uuid() WHERE id=$1`, in.ID)
	s.DB.QueryRow(ctx, `SELECT data_operation IS NOT NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&busy)
	current, _ := s.Inspect(ctx, in.ID, "")
	if _, e = s.Apply(ctx, "admin", Request{Kind: Enable, InstanceID: in.ID, ExpectedRevision: current.Instance.Revision, OperationKey: "reject-active-op", Fingerprint: d.Fingerprint, Confirm: true}); e == nil || !busy {
		t.Fatal("active data operation not fenced")
	}

}

func TestStateRecoveryAfterPersistedExchangeAndRestart(t *testing.T) {
	s, in, d := stateFixture(t)
	ctx := context.Background()
	path := filepath.Join(s.Root, in.ID, "data", "owned.txt")
	os.WriteFile(path, []byte("current"), 0600)
	snap, e := s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "restart-create", ""))
	if e != nil {
		t.Fatal(e)
	}
	task := jobs.UUID()
	dir := filepath.Join(s.Root, in.ID, "snapshots", task, "stage")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "owned.txt"), []byte("uncommitted"), 0600)
	r, e := s.stateRoot(in.ID)
	if e != nil {
		t.Fatal(e)
	}
	ino, e := identity(r, "snapshots/"+task+"/stage")
	if e != nil {
		t.Fatal(e)
	}
	original, e := identity(r, "data")
	if e != nil {
		t.Fatal(e)
	}
	info, _ := s.Inspect(ctx, in.ID, "")
	_, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint,stage_identity,reserved_bytes)VALUES($1,$2,$3,$4,$5,'restore','prepared',now()-interval '1 second',$6,$7,$8,83886080)`, task, in.ID, snap.ID, "restart-"+task, strings.Repeat("a", 64), info.Instance.Revision, d.Fingerprint, original+"|"+ino)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET data_operation=$2 WHERE id=$1`, in.ID, task); e != nil {
		t.Fatal(e)
	}
	if e = exchangeState(r, "data", "snapshots/"+task+"/stage"); e != nil {
		t.Fatal(e)
	}
	closeState(r)
	restarted := Store{DB: s.DB, Vault: s.Vault, Root: s.Root}
	if e = restarted.RecoverState(ctx); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "current" {
		t.Fatal("restart did not undo uncommitted exchange")
	}
	if _, e = os.Stat(filepath.Dir(dir)); !os.IsNotExist(e) {
		t.Fatal("orphan stage retained", e)
	}
}

func TestStateRestoreNeverRewindsHostBusinessReceipts(t *testing.T) {
	s, in, d := stateFixture(t)
	ctx := context.Background()
	ids := []string{}
	ciphers := [][]byte{}
	add := func(tag string) {
		t.Helper()
		id := jobs.UUID()
		cipher, e := s.seal("kv", id, json.RawMessage(`{"state":"unknown","effect":"must-not-replay"}`))
		if e != nil {
			t.Fatal(e)
		}
		_, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,1)`, id, in.ID, ".host/1700000000/"+strings.Repeat(tag, 64), cipher)
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
		ciphers = append(ciphers, cipher)
	}
	add("a")
	snap, e := s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "create", "receipts-create", ""))
	if e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_state_values WHERE snapshot_id=$1 AND kind='kv'`, snap.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("host-private keys entered snapshot", count, e)
	}
	add("b")
	if _, e = s.StateOperation(ctx, "admin", in.ID, stateRequest(t, s, in.ID, d, "restore", "receipts-restore", snap.ID)); e != nil {
		t.Fatal(e)
	}
	for n, id := range ids {
		var cipher []byte
		var revision int64
		if e = s.DB.QueryRow(ctx, `SELECT ciphertext,revision FROM plugin_v2_kv WHERE id=$1`, id).Scan(&cipher, &revision); e != nil || string(cipher) != string(ciphers[n]) || revision != 1 {
			t.Fatal("business receipt changed or disappeared", e)
		}
	}
}
