package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestI12HistoryFindsPendingConflictAndRetainsRecovery(t *testing.T) {
	f := newArchiveFixture(t)
	j := Journal{DB: f.db, Vault: f.e.Connections.Repository.Vault}
	stage := func(name string) string {
		t.Helper()
		tx, e := f.db.Begin(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(f.ctx)
		id, e := j.Stage(f.ctx, tx, f.root, name, f.asset, "asset:"+f.asset, "strm", f.raw)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(f.ctx); e != nil {
			t.Fatal(e)
		}
		return id
	}
	pending := stage("Pending.strm")
	conflict := stage("Conflict.strm")
	manual := filepath.Join(f.root, "Conflict.strm")
	if e := os.WriteFile(manual, []byte("manual content"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := j.Apply(f.ctx, conflict); !errors.Is(e, ErrArtifactConflict) {
		t.Fatal(e)
	}
	for _, c := range []struct{ state, id string }{{"pending", pending}, {"conflict", conflict}} {
		rows, total, e := f.e.History(f.ctx, HistoryFilter{Kind: "output", State: c.state, Limit: 25})
		if e != nil || total != 1 || len(rows) != 1 || rows[0].ID != c.id {
			t.Fatal("output not discoverable", c, rows, total, e)
		}
	}
	if e := j.Apply(f.ctx, pending); e != nil {
		t.Fatal(e)
	}
	rows, total, e := f.e.History(f.ctx, HistoryFilter{Kind: "output", State: "pending", Limit: 25})
	if e != nil || total != 0 || len(rows) != 0 {
		t.Fatal("retry did not settle pending", rows, total, e)
	}
	raw, e := os.ReadFile(manual)
	if e != nil || string(raw) != "manual content" {
		t.Fatal("manual file overwritten", e)
	}
	if e = os.Remove(manual); e != nil {
		t.Fatal(e)
	} // Only this test's synthetic file.
	if e = j.Apply(f.ctx, conflict); e != nil {
		t.Fatal(e)
	}
	rows, total, e = f.e.History(f.ctx, HistoryFilter{Kind: "output", State: "conflict", Limit: 25})
	if e != nil || total != 0 || len(rows) != 0 {
		t.Fatal("retry did not settle conflict", rows, total, e)
	}
	for _, p := range []string{f.next, f.archivePath, "Pending.strm", "Conflict.strm"} {
		raw, e := os.ReadFile(filepath.Join(f.root, p))
		if e != nil || string(raw) != string(f.raw) {
			t.Fatal("active or retired bytes changed", p, e)
		}
	}
}
