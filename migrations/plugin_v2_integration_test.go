package migrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/secrets"
)

func pluginStore(t *testing.T) (nativeFixture, v2.Store) {
	t.Helper()
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 64); e != nil {
		t.Fatal(e)
	}
	var search string
	if e := f.db.QueryRowContext(f.ctx, `SHOW search_path`).Scan(&search); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated test database required")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = search
	pool, e := pgxpool.NewWithConfig(f.ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	vault, e := secrets.New(strings.Repeat("21", 32))
	if e != nil {
		t.Fatal(e)
	}
	return f, v2.Store{DB: pool, Vault: vault, Root: filepath.Join(t.TempDir(), "plugins")}
}
func installRequest(t *testing.T) v2.Request {
	t.Helper()
	catalog, e := v2.Catalog()
	if e != nil || len(catalog) != 1 {
		t.Fatal(e)
	}
	d := catalog[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	return v2.Request{OperationKey: "install-test-one", Kind: v2.Install, PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes, Config: json.RawMessage(`{"owned":"synthetic"}`)}
}
func TestPluginV2MigrationArchiveDownUpPreservesOtherData(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 61); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO application_settings(singleton)VALUES(true) ON CONFLICT(singleton)DO UPDATE SET revision=17`)
	before := f.text(t, `SELECT md5(row_to_json(t)::text) FROM application_settings t`)
	unrelated := pluginUnrelatedSnapshot(t, f)
	f.exec(t, `INSERT INTO extension_plugins(id,name,version,manifest,wasm_module,config_ciphertext,plugin_data_ciphertext)VALUES('legacy-test','Old','1','{"runtime":"wasm"}',decode('0061736d01000000','hex'),decode('010203','hex'),decode('040506','hex'))`)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 62); e != nil {
			t.Fatal(e)
		}
		f.version(t, 62)
		if !reflect.DeepEqual(unrelated, pluginUnrelatedSnapshot(t, f)) {
			t.Fatal("non-plugin schema/data changed")
		}
		if got := f.text(t, `SELECT encode(config_ciphertext,'hex')||encode(plugin_data_ciphertext,'hex') FROM plugin_legacy_wasm_plugins WHERE id='legacy-test'`); got != "010203040506" {
			t.Fatal("legacy ciphertext changed")
		}
		if after := f.text(t, `SELECT md5(row_to_json(t)::text) FROM application_settings t`); after != before {
			t.Fatal("non-plugin data changed")
		}
		if _, e := f.provider.DownTo(f.ctx, 61); e != nil {
			t.Fatal(e)
		}
		f.version(t, 61)
	}
	if _, e := f.provider.UpTo(f.ctx, 62); e != nil {
		t.Fatal(e)
	}
}
func TestPluginV2InstancesHostCASFencesCloneAndLease(t *testing.T) {
	f, s := pluginStore(t)
	catalog, e := v2.Catalog()
	if e != nil || len(catalog) != 1 {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM plugin_v2_instances`); n != 0 {
		t.Fatal("catalog created instances")
	}
	if _, e = os.Stat(s.Root); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("catalog created data")
	}
	req := installRequest(t)
	bad := req
	bad.Fingerprint = strings.Repeat("0", 64)
	if _, e = s.Apply(f.ctx, "admin", bad); !errors.Is(e, v2.ErrConflict) {
		t.Fatal(e)
	}
	first, e := s.Apply(f.ctx, "admin", req)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Apply(f.ctx, "admin", req)
	if e != nil || first != again {
		t.Fatal("operation not idempotent", e)
	}
	changed := req
	changed.Config = json.RawMessage(`{}`)
	if _, e = s.Apply(f.ctx, "admin", changed); !errors.Is(e, v2.ErrConflict) {
		t.Fatal(e)
	}
	enable := v2.Request{OperationKey: "enable-test-one", Kind: v2.Enable, InstanceID: first.ID, ExpectedRevision: 1, Fingerprint: req.Fingerprint, Confirm: true}
	enabled, e := s.Apply(f.ctx, "admin", enable)
	if e != nil {
		t.Fatal(e)
	}
	var lease v2.Lease
	var mu sync.Mutex
	var claimed int
	var claimErr error
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, e := s.Claim(f.ctx, jobs.UUID())
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				lease = l
				claimed++
			} else if !errors.Is(e, pgx.ErrNoRows) {
				claimErr = e
			}
		}()
	}
	wg.Wait()
	if claimErr != nil || claimed != 1 {
		t.Fatalf("concurrent claim count=%d err=%v", claimed, claimErr)
	}
	if _, e = s.Claim(f.ctx, jobs.UUID()); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("duplicate lease", e)
	}
	host := s.Host(lease)
	call := func(method string, args any, revision *int64) (any, *contracts.Error) {
		raw, e := json.Marshal(args)
		if e != nil {
			t.Fatal(e)
		}
		body, e := json.Marshal(contracts.HostCall{Method: method, Args: raw, ExpectedRevision: revision})
		if e != nil {
			t.Fatal(e)
		}
		return host(f.ctx, body)
	}
	zero := int64(0)
	if _, rpc := call("plugin.data.put", map[string]any{"key": "owned", "value": "private-synthetic"}, &zero); rpc != nil {
		t.Fatal(rpc)
	}
	if _, rpc := call("plugin.data.put", map[string]any{"key": "owned", "value": 2}, &zero); rpc == nil || rpc.Code != contracts.Conflict {
		t.Fatal(rpc)
	}
	var cipher []byte
	if e = s.DB.QueryRow(f.ctx, `SELECT ciphertext FROM plugin_v2_kv WHERE instance_id=$1`, first.ID).Scan(&cipher); e != nil || bytes.Contains(cipher, []byte("private-synthetic")) {
		t.Fatal("not encrypted", e)
	}
	clone := req
	clone.Kind = v2.Clone
	clone.InstanceID = first.ID
	clone.ExpectedRevision = enabled.Revision
	clone.OperationKey = "clone-test-one"
	copy, e := s.Apply(f.ctx, "admin", clone)
	if e != nil {
		t.Fatal(e)
	}
	if copy.ID == first.ID || copy.Desired != v2.Disabled {
		t.Fatal(copy)
	}
	if f.integer(t, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1`, copy.ID) != 1 {
		t.Fatal("KV not cloned")
	}
	if _, rpc := call("notifications.send", contracts.Message{Title: "ungranted", Text: "must not queue"}, nil); rpc == nil || rpc.Code != contracts.PermissionDenied {
		t.Fatal("scope not enforced", rpc)
	}
	disable := enable
	disable.Kind = v2.Disable
	disable.OperationKey = "disable-test-one"
	disable.ExpectedRevision = enabled.Revision
	if _, e = s.Apply(f.ctx, "admin", disable); e != nil {
		t.Fatal(e)
	}
	if _, rpc := call("plugin.data.get", map[string]string{"key": "owned"}, nil); rpc == nil || rpc.Code != contracts.PermissionDenied {
		t.Fatal("revocation cached", rpc)
	}
	if e = s.Renew(f.ctx, lease, true); !errors.Is(e, v2.ErrConflict) {
		t.Fatal("late lease renewed", e)
	}
	if e = s.Finish(f.ctx, lease, v2.WorkerFailed); e != nil {
		t.Fatal(e)
	}
	if f.text(t, `SELECT runtime_state FROM plugin_v2_instances WHERE id=$1`, first.ID) != "stopped" {
		t.Fatal("controlled stop retried")
	}
	// Failed worker lease recovery, fencing and bounded deterministic backoff.
	enable.OperationKey = "enable-test-two"
	enable.ExpectedRevision = enabled.Revision + 1
	if _, e = s.Apply(f.ctx, "admin", enable); e != nil {
		t.Fatal(e)
	}
	stale, e := s.Claim(f.ctx, jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	f.exec(t, `UPDATE plugin_v2_instances SET lease_until=now()-interval '1 second' WHERE id=$1`, first.ID)
	if e = s.Reclaim(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(f.ctx, stale, v2.WorkerFailed); e == nil {
		t.Fatal("late finish accepted")
	}
	if v2.RetryDelay(0) != time.Second || v2.RetryDelay(1) != 5*time.Second || v2.RetryDelay(100) != 600*time.Second {
		t.Fatal("backoff")
	}
	if _, e := f.provider.DownTo(context.Background(), 61); e == nil {
		t.Fatal("destructive downgrade permitted")
	}
	t.Log(fmt.Sprintf("isolated plugin instances=%d", f.integer(t, `SELECT count(*) FROM plugin_v2_instances`)))
}

func pluginUnrelatedSnapshot(t *testing.T, f nativeFixture) map[string]string {
	t.Helper()
	rows, e := f.db.QueryContext(f.ctx, `SELECT table_name,string_agg(column_name||':'||data_type||':'||is_nullable||':'||coalesce(column_default,''),',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name NOT LIKE 'extension_%' AND table_name NOT LIKE 'plugin_%' AND table_name<>'goose_db_version' GROUP BY table_name ORDER BY table_name`)
	if e != nil {
		t.Fatal(e)
	}
	names := map[string]string{}
	for rows.Next() {
		var n, s string
		if e = rows.Scan(&n, &s); e != nil {
			t.Fatal(e)
		}
		names[n] = s
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	for name, columns := range names {
		var sum string
		if e = f.db.QueryRowContext(f.ctx, `SELECT md5(coalesce(string_agg(row_to_json(t)::text,',' ORDER BY row_to_json(t)::text),'')) FROM `+pgx.Identifier{name}.Sanitize()+` t`).Scan(&sum); e != nil {
			t.Fatal(e)
		}
		names[name] = columns + ":" + sum
	}
	return names
}
