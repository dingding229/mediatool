package migrations_test

import (
	"context"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func TestQRetirementGuardsAndPreservesLiveData(t *testing.T) {
	f, s := pluginStore(t)
	in, e := s.Apply(f.ctx, "admin", installRequest(t))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.provider.UpTo(f.ctx, 67); e != nil {
		t.Fatal(e)
	}
	// Exercise a populated schema-65 shape, not a brand-new empty database.
	site := "q-synthetic"
	f.exec(t, `INSERT INTO site_definitions(id,name,source_commit,definition)VALUES($1,'Synthetic','pinned','{}')`, site)
	task := jobs.UUID()
	f.exec(t, `INSERT INTO download_tasks(id,details)VALUES($1,'{"source":"synthetic"}')`, task)
	cipher, e := s.Vault.Seal("11111111-0000-4000-8000-000000000068", map[string]string{"accessToken": "synthetic-access", "refreshToken": "synthetic-refresh"})
	if e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES('11111111-0000-4000-8000-000000000068','open115','Synthetic',$1)`, cipher)
	f.exec(t, `INSERT INTO storage_connections(id,name,provider) VALUES(md5('q-storage')::uuid,'Synthetic','clouddrive2')`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes) VALUES(md5('q-file')::uuid,md5('q-storage')::uuid,'/Synthetic/movie.mkv',1)`)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id) VALUES(md5('q-asset')::uuid,md5('q-file')::uuid)`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference) VALUES(md5('q-strm')::uuid,md5('q-asset')::uuid,'Synthetic/movie.strm','synthetic-stable-resolver')`)
	strmBefore := f.text(t, `SELECT row_to_json(t)::text FROM strm_entries t WHERE id=md5('q-strm')::uuid`)
	before := f.text(t, `SELECT encode(credentials_ciphertext,'hex') FROM integrations WHERE id='11111111-0000-4000-8000-000000000068'`)
	pluginBefore := f.text(t, `SELECT encode(ciphertext,'hex') FROM plugin_v2_configs WHERE id=$1`, in.ID)
	tests := []struct{ name, setup, cleanup string }{
		{"bootstrap", `INSERT INTO bootstrap_imports(name)VALUES('synthetic')`, `DELETE FROM bootstrap_imports WHERE name='synthetic'`},
		{"wasm", `INSERT INTO plugin_legacy_wasm_plugins(id,name,version,manifest,wasm_module)VALUES('synthetic-old','Old','1','{"runtime":"wasm"}',decode('0061736d01000000','hex'))`, `DELETE FROM plugin_legacy_wasm_plugins WHERE id='synthetic-old'`},
		{"delivery", `INSERT INTO plugin_legacy_wasm_deliveries(event_id,plugin_id,state)VALUES(md5('old-delivery')::uuid,'synthetic-old','pending')`, `DELETE FROM plugin_legacy_wasm_deliveries WHERE plugin_id='synthetic-old'`},
		{"repository", `INSERT INTO plugin_legacy_wasm_repositories(id,name,url)VALUES(md5('old-repo')::uuid,'Synthetic','https://invalid.example')`, `DELETE FROM plugin_legacy_wasm_repositories WHERE name='Synthetic'`},
		{"site account", `INSERT INTO site_accounts(id,definition_id,name,credentials_ciphertext)VALUES(md5('old-account')::uuid,'q-synthetic','Synthetic','synthetic')`, `DELETE FROM site_accounts WHERE name='Synthetic'`},
		{"channel", `INSERT INTO notification_channels(id,name,kind)VALUES(md5('old-channel')::uuid,'Synthetic','synthetic')`, `DELETE FROM notification_channels WHERE name='Synthetic'`},
		{"legacy grants", `INSERT INTO media_users(id,username,display_name,password_hash) VALUES(md5('q-user')::uuid,'q_user','Synthetic','synthetic'); INSERT INTO media_user_legacy_grants(user_id,media_type) VALUES(md5('q-user')::uuid,'movie')`, `DELETE FROM media_user_legacy_grants WHERE user_id=md5('q-user')::uuid; DELETE FROM media_users WHERE id=md5('q-user')::uuid`},
		{"overrides", `UPDATE site_definitions SET local_overrides='{"credential":"do-not-erase"}' WHERE id='q-synthetic'`, `UPDATE site_definitions SET local_overrides='{}' WHERE id='q-synthetic'`},
	}
	for _, q := range tests {
		t.Run(q.name, func(t *testing.T) {
			f.exec(t, q.setup)
			if _, e = f.provider.UpTo(f.ctx, 68); e == nil {
				t.Fatal("nonempty legacy accepted")
			}
			f.version(t, 67)
			if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='download_tasks' AND column_name='site_account_id'`) != 1 {
				t.Fatal("partial DROP committed")
			}
			f.exec(t, q.cleanup)
		})
	}
	// An interrupted upgrade blocked on a writer must leave all tables/version intact.
	f.db.SetMaxOpenConns(3)
	tx, e := f.db.BeginTx(f.ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(f.ctx, `LOCK TABLE site_definitions IN ACCESS SHARE MODE`); e != nil {
		t.Fatal(e)
	}
	short, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if _, e = f.provider.UpTo(short, 68); e == nil {
		t.Fatal("lock interruption accepted")
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	f.db.SetMaxOpenConns(1)
	f.version(t, 67)
	if _, e = f.provider.UpTo(f.ctx, 68); e != nil {
		t.Fatal(e)
	}
	f.version(t, 68)
	if _, e = f.provider.UpTo(f.ctx, 68); e != nil {
		t.Fatal("retry", e)
	}
	if f.text(t, `SELECT encode(credentials_ciphertext,'hex') FROM integrations WHERE id='11111111-0000-4000-8000-000000000068'`) != before || f.text(t, `SELECT encode(ciphertext,'hex') FROM plugin_v2_configs WHERE id=$1`, in.ID) != pluginBefore {
		t.Fatal("ciphertext changed")
	}
	values, e := s.Vault.Open("11111111-0000-4000-8000-000000000068", cipher)
	if e != nil || values["accessToken"] != "synthetic-access" {
		t.Fatal("AAD/authorization changed", e)
	}
	if f.integer(t, `SELECT count(*) FROM download_tasks WHERE id=$1`, task) != 1 {
		t.Fatal("task erased")
	}
	if f.text(t, `SELECT row_to_json(t)::text FROM strm_entries t WHERE id=md5('q-strm')::uuid`) != strmBefore {
		t.Fatal("STRM resolver identity changed")
	}
	if _, e = f.provider.DownTo(f.ctx, 67); e == nil {
		t.Fatal("destructive downgrade allowed")
	}
	f.version(t, 68)
}

func TestQJournalMigrationBackfillsSourceReceipt(t *testing.T) {
	f, s := pluginStore(t)
	if _, e := f.provider.UpTo(f.ctx, 65); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload) VALUES(md5('pre-retention')::uuid,'files.succeeded',md5('pre-retention')::uuid,'{}')`)
	f.exec(t, `INSERT INTO plugin_v2_event_journal(event_id,event_type,payload) VALUES(md5('pre-retention')::uuid,'library.organized','{}')`)
	if _, e := f.provider.UpTo(f.ctx, 70); e != nil {
		t.Fatal(e)
	}
	f.version(t, 70)
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE plugin_journal_cursor IS NOT NULL`) != 1 {
		t.Fatal("old journal identity not fenced")
	}
	if e := s.Bridge(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM plugin_v2_event_journal`) != 1 {
		t.Fatal("upgraded event replayed")
	}
}
