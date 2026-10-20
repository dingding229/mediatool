package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQ6ReadOnlyPreflightRejectsBadStateAndNeverWrites(t *testing.T) {
	f, s := pluginStore(t)
	if _, e := f.provider.UpTo(f.ctx, 65); e != nil {
		t.Fatal(e)
	}
	in, e := s.Apply(f.ctx, "admin", installRequest(t))
	if e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO plugin_v2_state_snapshots(id,instance_id,plugin_id,fingerprint,config_schema,data_schema,operation_revision,state,bytes,data_path)VALUES(md5('preflight-snap')::uuid,$1,'synthetic',repeat('a',64),1,1,1,'ready',0,'snapshots/11111111-1111-1111-1111-111111111111/data')`, in.ID)
	f.exec(t, `INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint)VALUES(md5('preflight-task')::uuid,$1,md5('preflight-snap')::uuid,'q6-preflight',repeat('b',64),'create','writing',now(),1,repeat('a',64))`, in.ID)
	raw, e := os.ReadFile(filepath.Join("..", "docs", "sql", "20261020-q6-preflight.sql"))
	if e != nil {
		t.Fatal(e)
	}
	var schema string
	if e = f.db.QueryRowContext(f.ctx, `SELECT current_schema()`).Scan(&schema); e != nil {
		t.Fatal(e)
	}
	// Namespace substitution is test-only, to the fixture's owned schema. All
	// predicates, READ ONLY, RAISE conditions and the expected version stay intact.
	sql := strings.ReplaceAll(string(raw), "public.", schema+".")
	sql = strings.ReplaceAll(sql, ":'expected_database'", "'media_tool_test'")
	run := func() error {
		_, e := f.db.ExecContext(f.ctx, sql)
		_, cleanup := f.db.ExecContext(context.Background(), `ROLLBACK`)
		if cleanup != nil {
			t.Fatal(cleanup)
		}
		return e
	}
	before := f.text(t, `SELECT md5(row_to_json(t)::text) FROM plugin_v2_state_tasks t`)
	if e = run(); e != nil {
		t.Fatal(e)
	}
	if f.text(t, `SELECT md5(row_to_json(t)::text) FROM plugin_v2_state_tasks t`) != before {
		t.Fatal("preflight changed data")
	}
	f.exec(t, `UPDATE plugin_v2_state_tasks SET revision=0 WHERE operation_key='q6-preflight'`)
	if e = run(); e == nil {
		t.Fatal("bad historical state allowed")
	}
	if f.integer(t, `SELECT revision FROM plugin_v2_state_tasks WHERE operation_key='q6-preflight'`) != 0 {
		t.Fatal("preflight fixed data instead of rejecting")
	}
	f.exec(t, `UPDATE plugin_v2_state_tasks SET revision=1 WHERE operation_key='q6-preflight'`)
	f.exec(t, `INSERT INTO bootstrap_imports(name)VALUES('must-not-delete')`)
	if e = run(); e == nil {
		t.Fatal("nonempty legacy allowed")
	}
	if f.integer(t, `SELECT count(*) FROM bootstrap_imports`) != 1 {
		t.Fatal("preflight deleted legacy")
	}
	f.version(t, 65)
}
