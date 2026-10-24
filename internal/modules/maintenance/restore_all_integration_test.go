package maintenance

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/secrets"
)

// No second database. Immutable target and snapshot archives are restored into
// media_tool_test with the exact production data-only importer; the only seam
// avoids CREATE DATABASE by validating ciphertext via a transaction dry run.
func allBusinessFixture(t *testing.T) (Engine, string, string, string, string) {
	e, _ := webFixture(t)
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("test lock required")
	}
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := e.DB.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`TRUNCATE administrator_accounts,integrations,storage_connections,cloud_files,media_assets,media_items,jobs,outbox_events,plugin_v2_definitions,external_api_keys CASCADE`)
	t.Cleanup(func() {
		e.DB.Exec(context.Background(), `TRUNCATE administrator_accounts,integrations,storage_connections,cloud_files,media_assets,media_items,jobs,outbox_events,plugin_v2_definitions,external_api_keys CASCADE`)
	})
	var originalSettings []byte
	var originalRevision int64
	var originalUpdated time.Time
	if err := e.DB.QueryRow(ctx, `SELECT value,revision,updated_at FROM application_settings WHERE singleton`).Scan(&originalSettings, &originalRevision, &originalUpdated); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := e.DB.Exec(context.Background(), `UPDATE application_settings SET value=$1,revision=$2,updated_at=$3 WHERE singleton`, originalSettings, originalRevision, originalUpdated); err != nil {
			t.Error(err)
		}
	})
	account, err := (accounts.Repository{DB: e.DB}).Register(ctx, "restore_owner", "BackupPassword1", "backup-cookie")
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := secrets.New(e.Key)
	connection, authorization, plugin := newID(), newID(), newID()
	cipher, _ := vault.Seal(connection, map[string]string{"accessToken": "backup-115-token"})
	exec(`INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'open115','restore-fixture',$2)`, connection, cipher)
	payload, _ := vault.Seal("open115-auth:"+authorization, map[string]string{"verifier": "backup-verifier"})
	exec(`INSERT INTO open115_authorizations(id,integration_id,owner_account_id,owner_version,integration_revision,state,payload_ciphertext,expires_at)VALUES($1,$2,$3,1,1,'pending',$4,now()+interval '1 hour')`, authorization, connection, account.ID, payload)
	exec(`UPDATE application_settings SET value='{"movieTemplate":"Backup/{title} ({year})/{title}"}',revision=1 WHERE singleton`)
	exec(`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',733001,'Backup film','{}')`, newID())
	exec(`INSERT INTO plugin_v2_definitions(id,manifest,fingerprint)VALUES('b4b-fixture','{}',$1)`, strings.Repeat("a", 64))
	exec(`INSERT INTO plugin_v2_instances(id,plugin_id,desired_state,approved_fingerprint)VALUES($1,'b4b-fixture','enabled',$2)`, plugin, strings.Repeat("a", 64))
	pc, _ := vault.Seal("plugin:v2:config:"+plugin, map[string]string{"value": "backup-config"})
	exec(`INSERT INTO plugin_v2_configs(id,ciphertext)VALUES($1,$2)`, plugin, pc)
	kv := newID()
	kc, _ := vault.Seal("plugin:v2:kv:"+kv, map[string]string{"value": "backup-kv"})
	exec(`INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,'fixture',$3,1)`, kv, plugin, kc)
	// Restore the fixture's seeded baseline at cleanup so empty-source coverage
	// does not leak missing singleton rows into another serial integration suite.
	target := newID()
	exec(`INSERT INTO ops_backups(id,origin)VALUES($1,'fixture-all')`, target)
	if err = e.Backup(ctx, target); err != nil {
		t.Fatal(err)
	}
	nextVault, _ := secrets.New(strings.Repeat("8", 64))
	if _, err := secrets.Rekey(ctx, e.DB, vault, nextVault, true); err != nil {
		t.Fatal("fixture backup ciphertext proof", err)
	}
	t.Cleanup(func() {
		dir, _ := e.dir(target)
		if err := e.importWebDump(context.Background(), filepath.Join(dir, "database.dump"), newID(), target); err != nil {
			t.Error("fixture baseline cleanup", err)
		}
	})
	e.validateRestore = func(context.Context, Manifest, string) error { return nil }
	e.prepareRestore = func(ctx context.Context, r WebRestore, m Manifest, dir string) (string, func() error, error) {
		return filepath.Join(dir, "database.dump"), nil, nil
	}
	// Keep the immutable source values for the assertions below; no secrets leave fixtures.
	return e, target, connection, account.ID, plugin
}
func TestRestoreAllBusinessAndReturnToPreSnapshot(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed", true: "empty"}[empty], func(t *testing.T) {
			e, target, connection, account, plugin := allBusinessFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := e.DB.Exec(ctx, q, args...); err != nil {
					t.Fatal(err)
				}
			}
			vault, _ := secrets.New(e.Key)
			changed, _ := vault.Seal(connection, map[string]string{"accessToken": "current-115-token"})
			exec(`UPDATE integrations SET credentials_ciphertext=$2,revision=2 WHERE id=$1`, connection, changed)
			exec(`UPDATE application_settings SET value='{"movieTemplate":"Current/{title} ({year})/{title}"}',revision=2 WHERE singleton`)
			passwordHash, hashErr := auth.HashPassword("CurrentPassword2")
			if hashErr != nil {
				t.Fatal(hashErr)
			}
			exec(`UPDATE administrator_accounts SET username='current_owner',credential_version=2,password_hash=$2 WHERE id=$1`, account, passwordHash)
			var kv string
			if err := e.DB.QueryRow(ctx, `SELECT id::text FROM plugin_v2_kv WHERE instance_id=$1`, plugin).Scan(&kv); err != nil {
				t.Fatal(err)
			}
			currentKV, _ := vault.Seal("plugin:v2:kv:"+kv, map[string]string{"value": "current-kv"})
			exec(`UPDATE plugin_v2_kv SET ciphertext=$2,revision=2 WHERE id=$1`, kv, currentKV)
			exec(`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',733002,'Added after backup','{}')`, newID())
			pc, _ := vault.Seal("plugin:v2:config:"+plugin, map[string]string{"value": "current-config"})
			exec(`UPDATE plugin_v2_configs SET ciphertext=$2,revision=2 WHERE id=$1`, plugin, pc)
			// A valid post-backup session must be invalid after the atomic switch.
			var a accounts.Account
			if err := e.DB.QueryRow(ctx, `SELECT id::text,username,credential_version FROM administrator_accounts WHERE id=$1`, account).Scan(&a.ID, &a.Username, &a.Version); err != nil {
				t.Fatal(err)
			}
			if _, err := (accounts.Repository{DB: e.DB}).CreateSession(ctx, a, "current-cookie"); err != nil {
				t.Fatal(err)
			}
			if empty {
				order, err := e.restoreOrder(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for i := len(order) - 1; i >= 0; i-- {
					exec("DELETE FROM " + quote(order[i]))
				}
			}
			before, err := Protected(ctx, e.DB)
			if err != nil {
				t.Fatal(err)
			}
			r, err := e.PreviewWebRestore(ctx, target)
			if err != nil {
				t.Fatal("different or empty business must restore", err)
			}
			changedRow := false
			for _, v := range r.TableChanges {
				if v.Name == "media_items" && v.CurrentRows != v.BackupRows {
					changedRow = true
				}
			}
			if !changedRow {
				t.Fatal("missing row diff")
			}
			r, err = e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.RunWebRestore(ctx); err != nil {
				t.Fatal(err)
			}
			r, err = e.WebRestore(ctx, r.ID)
			if err != nil || r.State != "completed" || r.SnapshotID == nil {
				t.Fatal(r.State, r.Message, err)
			}
			var value string
			if err = e.DB.QueryRow(ctx, `SELECT value->>'movieTemplate'FROM application_settings`).Scan(&value); err != nil || value != "Backup/{title} ({year})/{title}" {
				t.Fatal(value, err)
			}
			var raw []byte
			if err = e.DB.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, connection).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			v, err := vault.Open(connection, raw)
			if err != nil || v["accessToken"] != "backup-115-token" {
				t.Fatal("115 not restored", err)
			}
			if err = e.DB.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_configs WHERE id=$1`, plugin).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			v, err = vault.Open("plugin:v2:config:"+plugin, raw)
			if err != nil || v["value"] != "backup-config" {
				t.Fatal("plugin not restored", err)
			}
			if _, err = (accounts.Repository{DB: e.DB}).Authenticate(ctx, "restore_owner", "BackupPassword1"); err != nil {
				t.Fatal("backup password not restored", err)
			}
			if err = e.DB.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_kv WHERE id=$1`, kv).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			v, err = vault.Open("plugin:v2:kv:"+kv, raw)
			if err != nil || v["value"] != "backup-kv" {
				t.Fatal("KV not restored", err)
			}
			digest := sha256.Sum256([]byte("current-cookie"))
			if _, err = (accounts.Repository{DB: e.DB}).Session(ctx, digest[:]); err == nil {
				t.Fatal("current session survived")
			}
			rollback := queueWeb(t, e, *r.SnapshotID)
			if err = e.RunWebRestore(ctx); err != nil {
				t.Fatal(err)
			}
			result, err := e.WebRestore(ctx, rollback.ID)
			if err != nil || result.State != "completed" {
				t.Fatal(result.State, result.Message, err)
			}
			after, err := Protected(ctx, e.DB)
			if err != nil || !sameProtected(before, after) {
				t.Fatal("rollback did not restore all current values", err)
			}
			if !empty {
				if _, err = (accounts.Repository{DB: e.DB}).Authenticate(ctx, "current_owner", "CurrentPassword2"); err != nil {
					t.Fatal("current password not restored", err)
				}
				if err = e.DB.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_configs WHERE id=$1`, plugin).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				v, err = vault.Open("plugin:v2:config:"+plugin, raw)
				if err != nil || v["value"] != "current-config" {
					t.Fatal("current plugin config not restored", err)
				}
				if err = e.DB.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_kv WHERE id=$1`, kv).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				v, err = vault.Open("plugin:v2:kv:"+kv, raw)
				if err != nil || v["value"] != "current-kv" {
					t.Fatal("current KV not restored", err)
				}
			}

		})
	}
}
func TestRestoreRejectsEveryUnreadableCiphertextWithoutChangingIt(t *testing.T) {
	e, _, _, _, plugin := allBusinessFixture(t)
	ctx := context.Background()
	if _, err := e.DB.Exec(ctx, `UPDATE plugin_v2_configs SET ciphertext=decode(repeat('ab',28),'hex')WHERE id=$1`, plugin); err != nil {
		t.Fatal(err)
	}
	bad := newID()
	if _, err := e.DB.Exec(ctx, `INSERT INTO ops_backups(id,origin)VALUES($1,'unreadable-cipher-fixture')`, bad); err != nil {
		t.Fatal(err)
	}
	if err := e.Backup(ctx, bad); err != nil {
		t.Fatal(err)
	}
	e.validateRestore = func(ctx context.Context, _ Manifest, _ string) error {
		old, _ := secrets.New(e.Key)
		next, _ := secrets.New(strings.Repeat("8", 64))
		_, err := secrets.Rekey(ctx, e.DB, old, next, true)
		return err
	}
	if _, err := e.PreviewWebRestore(ctx, bad); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatal("unreadable cipher admitted", err)
	}
}
func TestRestoreAtomicAuthAndProgressReadableUntilCommit(t *testing.T) {
	e, target, _, account, _ := allBusinessFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := e.DB.Exec(ctx, `UPDATE administrator_accounts SET username='during_restore',credential_version=2 WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := (accounts.Repository{DB: e.DB}).CreateSession(ctx, accounts.Account{ID: account, Username: "during_restore", Version: 2}, "pending-current-cookie"); err != nil {
		t.Fatal(err)
	}
	r := queueWeb(t, e, target)
	base := e.Run
	entered := make(chan struct{})
	resume := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(resume) }) }
	defer release()
	e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		if name == "psql" {
			in = io.MultiReader(strings.NewReader(""), in)
			b := &restoreCommitBarrier{source: in, entered: entered, resume: resume}
			return base(ctx, name, args, env, b, out)
		}
		return base(ctx, name, args, env, in, out)
	}
	done := make(chan error, 1)
	go func() { done <- e.RunWebRestore(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("import never reached barrier")
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var idle bool
		if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name='mediatool-maintenance' AND state='idle in transaction')`).Scan(&idle); err != nil {
			t.Fatal(err)
		}
		if idle {
			break
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("COPY did not reach transaction boundary")
		}
	}
	var name string
	read, c := context.WithTimeout(ctx, time.Second)
	defer c()
	if err := e.DB.QueryRow(read, `SELECT username FROM administrator_accounts WHERE id=$1`, account).Scan(&name); err != nil || name != "during_restore" {
		t.Fatal("auth read blocked", err)
	}
	digest := sha256.Sum256([]byte("pending-current-cookie"))
	if _, err := (accounts.Repository{DB: e.DB}).Session(read, digest[:]); err != nil {
		t.Fatal("current auth unavailable before commit", err)
	}
	if _, err := e.WebRestores(read); err != nil {
		t.Fatal("progress blocked", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := (accounts.Repository{DB: e.DB}).Session(ctx, digest[:]); err == nil {
		t.Fatal("old auth remained valid after commit")
	}
	result, err := e.WebRestore(ctx, r.ID)
	if err != nil || result.State != "completed" {
		t.Fatal(result.State, result.Message, err)
	}
}

// Pause after COPY has been consumed but before psql receives EOF/COMMIT.
type restoreCommitBarrier struct {
	source  io.Reader
	entered chan struct{}
	resume  chan struct{}
	done    bool
}

func (b *restoreCommitBarrier) Read(p []byte) (int, error) {
	n, err := b.source.Read(p)
	if err == io.EOF && !b.done {
		b.done = true
		close(b.entered)
		<-b.resume
	}
	return n, err
}

func TestApprovedCLIRestoreUsesSameSnapshotAndReceiptOnce(t *testing.T) {
	e, target := webFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := e.DB.Exec(ctx, `INSERT INTO ops_drills(id,backup_id,state,completed_at)VALUES($1,$2,'verified',now())`, newID(), target); err != nil {
		t.Fatal(err)
	}
	req, err := e.Preview(ctx, "restore", target)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Approve(ctx, req.ID, req.Digest, ConfirmText(req)); err != nil {
		t.Fatal(err)
	}
	first, err := e.RestoreApprovedRequest(ctx, req.ID)
	if err != nil || first.State != "completed" {
		t.Fatal(first.State, first.Message, err)
	}
	second, err := e.RestoreApprovedRequest(ctx, req.ID)
	if err != nil || second.ID != first.ID || second.SnapshotID == nil || *second.SnapshotID != *first.SnapshotID {
		t.Fatal("CLI replay", second, err)
	}
	var n int
	if err = e.DB.QueryRow(ctx, `SELECT count(*)FROM ops_backups WHERE origin='pre-restore'`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err = e.VerifyExecution(ctx, req.ID); err != nil {
		t.Fatal("restored expected values not verifiable", err)
	}
}

func TestRestoreRuntimeNeverReplaysOldPluginEventsOrDiskTasks(t *testing.T) {
	e, _, _, _, plugin := allBusinessFixture(t)
	ctx := context.Background()
	snapshot, stateSnapshot, task := newID(), newID(), newID()
	fp := strings.Repeat("a", 64)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := e.DB.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO plugin_v2_snapshots(id,instance_id,generation,fingerprint,descriptor_hash,descriptors,grant_revision)VALUES($1,$2,$4,$3,$3,'{}',1)`, snapshot, plugin, fp, newID())
	exec(`INSERT INTO plugin_v2_event_journal(event_id,event_type,payload)VALUES($1,'fixture.restore','{}')`, newID())
	exec(`INSERT INTO plugin_v2_subscriptions(instance_id,handler,event_type,snapshot_id,cursor)VALUES($1,'fixture','fixture.restore',$2,0)`, plugin, snapshot)
	exec(`INSERT INTO plugin_v2_state_snapshots(id,instance_id,plugin_id,fingerprint,config_schema,data_schema,operation_revision,state,bytes,data_path)VALUES($1,$2,'b4b-fixture',$3,1,1,1,'writing',0,$4)`, stateSnapshot, plugin, fp, "snapshots/"+stateSnapshot+"/data")
	exec(`INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint)VALUES($1,$2,$3,$4,$5,'create','writing',now()+interval '1 hour',1,$5)`, task, plugin, stateSnapshot, "restore-fixture:"+task, fp)
	exec(restoreRuntimeSQL)
	var cursor, max int64
	if err := e.DB.QueryRow(ctx, `SELECT cursor,(SELECT max(cursor)FROM plugin_v2_event_journal)FROM plugin_v2_subscriptions WHERE instance_id=$1`, plugin).Scan(&cursor, &max); err != nil || cursor != max {
		t.Fatal(cursor, max, err)
	}
	var state string
	var cleaned bool
	if err := e.DB.QueryRow(ctx, `SELECT state,cleaned FROM plugin_v2_state_tasks WHERE id=$1`, task).Scan(&state, &cleaned); err != nil || state != "failed" || !cleaned {
		t.Fatal(state, cleaned, err)
	}
}

func TestRestoreAllAllowsDamagedCurrentAndCompensation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "compensate"}[fail], func(t *testing.T) {
			e, target, connection, _, _ := allBusinessFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			bad := []byte("already-damaged-current-cipher")
			if _, err := e.DB.Exec(ctx, `UPDATE integrations SET credentials_ciphertext=$2 WHERE id=$1`, connection, bad); err != nil {
				t.Fatal(err)
			}
			r := queueWeb(t, e, target)
			if fail {
				base := e.Run
				calls := 0
				e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
					err := base(ctx, name, args, env, in, out)
					if name == "psql" {
						calls++
						if calls == 1 && err == nil {
							return errors.New("lost post-commit acknowledgement")
						}
					}
					return err
				}
			}
			if err := e.RunWebRestore(ctx); err != nil {
				t.Fatal(err)
			}
			result, err := e.WebRestore(ctx, r.ID)
			if err != nil || result.SnapshotID == nil {
				t.Fatal(result, err)
			}
			var raw []byte
			if err = e.DB.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, connection).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if fail {
				if result.State != "rolled_back" || string(raw) != string(bad) {
					t.Fatal("damaged pre-state was not returned", result.State)
				}
			} else {
				vault, _ := secrets.New(e.Key)
				v, err := vault.Open(connection, raw)
				if result.State != "completed" || err != nil || v["accessToken"] != "backup-115-token" {
					t.Fatal("valid backup could not replace damaged source", result.State, err)
				}
			}
		})
	}
}
