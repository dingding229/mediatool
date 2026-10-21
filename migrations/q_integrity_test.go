package migrations_test

import (
	"encoding/json"
	"media-tool/internal/modules/jobs"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/migrations"
	"strings"
	"testing"
)

func TestQIndexesSyntheticPlansAndRestart(t *testing.T) {
	f, s := pluginStoreAt(t, 64)
	in, e := historicalInstall(f, s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 65); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO jobs(id,kind,payload) SELECT md5('job'||n)::uuid,'plugin.v2.service',jsonb_build_object('instanceId',CASE WHEN n<=100 THEN $1 ELSE md5(n::text) END,'serviceId','tick','snapshotId','synthetic') FROM generate_series(1,40000) n`, in.ID)
	f.exec(t, `INSERT INTO plugin_v2_manual_requests(key,instance_id,request_hash,job_id) SELECT 'q-'||n,$1,'synthetic',md5('job'||n)::uuid FROM generate_series(1,40000) n`, in.ID)
	f.exec(t, `INSERT INTO plugin_v2_snapshots(id,instance_id,generation,grant_revision,fingerprint,descriptor_hash,descriptors) VALUES(md5('snap')::uuid,$1,md5('generation')::uuid,1,repeat('a',64),repeat('b',64),'{}')`, in.ID)
	f.exec(t, `INSERT INTO plugin_v2_runs(id,instance_id,snapshot_id,generation,grant_revision,kind,handler,dedupe_key,request_hash,args_ciphertext,deadline_at) SELECT md5('run'||n)::uuid,$1,md5('snap')::uuid,md5('generation')::uuid,1,'action','synthetic','workflow:'||CASE WHEN n<=50 THEN 'target' ELSE n::text END||':'||n,'synthetic','synthetic',now()+interval '1 hour' FROM generate_series(1,40000) n`, in.ID)
	f.exec(t, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload) SELECT md5('event'||n)::uuid,CASE WHEN n<=100 THEN 'site.metrics.refreshed' ELSE 'unrelated' END,md5('aggregate')::uuid,'{}' FROM generate_series(1,40000) n`)
	queries := []struct {
		name, sql, index string
		args             []any
	}{
		{"owned jobs", `SELECT j.id FROM jobs j WHERE j.payload->>'instanceId'=$1 AND EXISTS(SELECT 1 FROM plugin_v2_manual_requests m WHERE m.job_id=j.id AND m.instance_id::text=$1) AND($2=''OR j.kind=$2)AND($3=''OR j.state=$3)AND($4=''OR j.id::text=$4)ORDER BY j.created_at DESC,j.id LIMIT 101 OFFSET 0`, "q_jobs_plugin_owner", []any{in.ID, "", "", ""}},
		{"manual ownership", `SELECT 1 FROM plugin_v2_manual_requests WHERE job_id=md5('job1')::uuid AND instance_id::text=$1`, "q_manual_job_owner", []any{in.ID}},
		{"service", `SELECT id FROM jobs WHERE kind='plugin.v2.service' AND payload->>'instanceId'=$1 AND payload->>'serviceId'='tick' AND payload->>'snapshotId'='synthetic' AND(state IN('pending','running','retry') OR EXISTS(SELECT 1 FROM plugin_v2_runs r WHERE r.dedupe_key='service-job:'||jobs.id::text AND r.state IN('pending','running'))) ORDER BY created_at LIMIT 1`, "q_jobs_plugin_service", []any{in.ID}},
		{"workflow", `SELECT dedupe_key,state,reason FROM plugin_v2_runs WHERE dedupe_key LIKE 'workflow:target:%' ORDER BY created_at LIMIT 32`, "q_runs_workflow", nil},
	}
	// Use the exact Bridge predicate (not a favorable shortened source query).
	raw, _ := migrationsSQL("00066_query_indexes.sql")
	start := strings.Index(raw, "WHERE event_type IN(")
	end := strings.Index(raw[start:], ");")
	predicate := raw[start : start+end+1]
	queries = append(queries, struct {
		name, sql, index string
		args             []any
	}{"bridge", `SELECT o.id FROM outbox_events o ` + strings.Replace(predicate, "event_type", "o.event_type", 1) + ` AND NOT EXISTS(SELECT 1 FROM plugin_v2_event_journal j WHERE j.event_id=o.id) ORDER BY o.created_at,o.id LIMIT 200`, "q_outbox_bridge", nil})
	if _, e = f.db.ExecContext(f.ctx, `ANALYZE`); e != nil {
		t.Fatal(e)
	}
	for _, q := range queries {
		t.Log("BEFORE", q.name, explainQ(t, f, q.sql, q.args...))
	}
	// Simulate an interrupted concurrent build left before goose records 66.
	if _, e = f.db.ExecContext(f.ctx, `CREATE INDEX q_jobs_plugin_owner ON jobs(id)`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 67); e != nil {
		t.Fatal(e)
	}
	f.version(t, 67)
	for _, q := range queries {
		plan := explainQ(t, f, q.sql, q.args...)
		t.Log("AFTER", q.name, plan)
		if !strings.Contains(plan, q.index) {
			t.Errorf("expected %s: %s", q.index, plan)
		}
	}
	if f.integer(t, `SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname='plugin_v2_state_values_snapshot'`) != 0 {
		t.Fatal("redundant index retained")
	}
	if _, e = f.provider.DownTo(f.ctx, 65); e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 67); e != nil {
		t.Fatal(e)
	}
}
func explainQ(t *testing.T, f nativeFixture, q string, args ...any) string {
	t.Helper()
	var raw []byte
	if e := f.db.QueryRowContext(f.ctx, `EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) `+q, args...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var compact any
	if e := json.Unmarshal(raw, &compact); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(compact)
	return string(b)
}
func migrationsSQL(name string) (string, error) {
	b, e := migrations.FS.ReadFile(name)
	return string(b), e
}

func TestQStateAndJSONConstraints(t *testing.T) {
	f, s := pluginStoreAt(t, 64)
	in, e := historicalInstall(f, s)
	if e != nil {
		t.Fatal(e)
	}
	q := installRequest(t)
	q.OperationKey = "other-install"
	other, e := historicalInstall(f, s)
	_ = q
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 67); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO plugin_v2_state_snapshots(id,instance_id,plugin_id,fingerprint,config_schema,data_schema,operation_revision,state,bytes,data_path) VALUES(md5('state')::uuid,$1,'synthetic',repeat('a',64),1,1,1,'ready',0,'snapshots/11111111-1111-1111-1111-111111111111/data')`, in.ID)
	sql := `INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint) VALUES(md5('task')::uuid,$1,md5('state')::uuid,'q-task',repeat('b',64),'create','writing',now(),$2,$3)`
	f.rejects(t, "23503", sql, other.ID, 1, strings.Repeat("a", 64))
	f.rejects(t, "23514", sql, in.ID, 0, strings.Repeat("a", 64))
	f.rejects(t, "23514", sql, in.ID, 1, "bad")
	f.exec(t, sql, in.ID, 1, strings.Repeat("a", 64))
	f.rejects(t, "23514", `INSERT INTO search_runs(id,keyword,account_ids) VALUES(md5('search')::uuid,'synthetic','{}')`)
	f.rejects(t, "23514", `INSERT INTO subscriptions(id,title,media_type,rules) VALUES(md5('subscription')::uuid,'synthetic','movie','[]')`)
	f.rejects(t, "23514", `INSERT INTO download_tasks(id,details) VALUES(md5('download')::uuid,'[]')`)
}

func TestQInvalidHistoricalStateAbortsValidation(t *testing.T) {
	f, s := pluginStoreAt(t, 64)
	in, e := historicalInstall(f, s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 66); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO plugin_v2_state_snapshots(id,instance_id,plugin_id,fingerprint,config_schema,data_schema,operation_revision,state,bytes,data_path) VALUES(md5('state')::uuid,$1,'synthetic',repeat('a',64),1,1,1,'ready',0,'snapshots/11111111-1111-1111-1111-111111111111/data')`, in.ID)
	f.exec(t, `INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint) VALUES(md5('task')::uuid,$1,md5('state')::uuid,'q-task',repeat('b',64),'create','writing',now(),0,'bad')`, in.ID)
	if _, e = f.provider.UpTo(f.ctx, 67); e == nil {
		t.Fatal("invalid historical row accepted")
	}
	f.version(t, 66)
	if f.text(t, `SELECT fingerprint FROM plugin_v2_state_tasks WHERE operation_key='q-task'`) != "bad" {
		t.Fatal("migration rewrote old identity")
	}
	f.exec(t, `UPDATE plugin_v2_state_tasks SET revision=1,fingerprint=repeat('a',64) WHERE operation_key='q-task'`)
	if _, e = f.provider.UpTo(f.ctx, 67); e != nil {
		t.Fatal(e)
	}
}

// Historical schema tests seed their historical representation directly. They
// must not run today's content-fingerprint installer against schema64.
func historicalInstall(f nativeFixture, s v2.Store) (v2.Instance, error) {
	id := jobs.UUID()
	if _, e := f.db.ExecContext(f.ctx, `INSERT INTO plugin_v2_definitions(id,manifest,fingerprint)VALUES('synthetic','{}',repeat('a',64)) ON CONFLICT DO NOTHING`); e != nil {
		return v2.Instance{}, e
	}
	_, e := f.db.ExecContext(f.ctx, `INSERT INTO plugin_v2_instances(id,plugin_id,approved_fingerprint)VALUES($1,'synthetic',repeat('a',64))`, id)
	if e != nil {
		return v2.Instance{}, e
	}
	cipher, e := s.Vault.Seal("plugin:v2:config:"+id, map[string]string{"value": "{}"})
	if e != nil {
		return v2.Instance{}, e
	}
	_, e = f.db.ExecContext(f.ctx, `INSERT INTO plugin_v2_configs(id,ciphertext)VALUES($1,$2)`, id, cipher)
	return v2.Instance{ID: id}, e
}
