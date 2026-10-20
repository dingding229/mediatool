package maintenance

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func fixture(t *testing.T) Engine {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	cleanup := `DELETE FROM jobs WHERE kind LIKE 'ops.%';DELETE FROM outbox_events WHERE event_type LIKE 'ops.%';TRUNCATE ops_requests,ops_drills,ops_backups,ops_releases,ops_alerts,ops_samples CASCADE;UPDATE ops_policy SET value='{}',revision=1,next_sample_at=now(),next_backup_at=now()+interval '1 day' WHERE singleton;UPDATE ops_mode SET paused=false,reason='',revision=1 WHERE singleton;`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(ctx, cleanup) })
	root := t.TempDir()
	out := t.TempDir()
	return Engine{DB: db, DSN: dsn, Root: root, OutputRoot: out, Key: strings.Repeat("9", 64), Revision: strings.Repeat("a", 40), HostProc: t.TempDir()}
}
func TestPoliciesSchedulingAlertsRetentionAndApprovals(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	r, err := e.Policy(ctx)
	if err != nil || r.Value.KeepCount != 10 {
		t.Fatal(r, err)
	}
	r.Value.KeepCount = 1
	if _, err = e.SavePolicy(ctx, r); err == nil {
		t.Fatal("unsafe retention accepted")
	}
	r.Value = Defaults()
	r.Value.BackupEnabled = true
	r.Value.BackupCron = "*/5 * * * *"
	if _, err = e.SavePolicy(ctx, r); err == nil {
		t.Fatal("too frequent backup")
	}
	r.Value.BackupCron = "0 3 * * *"
	saved, err := e.SavePolicy(ctx, r)
	if err != nil || saved.Revision != 2 {
		t.Fatal(saved, err)
	}
	if _, err = e.SavePolicy(ctx, r); err != ErrConflict {
		t.Fatal(err)
	}
	p := Defaults()
	p.Consecutive = 2
	p.FailedJobs = 3
	v := map[string]any{"failedJobs10m": 4, "memoryPercent": 10.0, "diskFreeGB": 100.0, "loadPerCPU": 0.1, "offlineQueues": 0}
	for i := 0; i < 4; i++ {
		if err = e.evaluate(ctx, p, v); err != nil {
			t.Fatal(err)
		}
	}
	var events int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='ops.alert.active'`).Scan(&events)
	if events != 1 {
		t.Fatal("alert not deduplicated", events)
	}
	defer e.DB.Exec(ctx, `DELETE FROM outbox_events WHERE event_type LIKE 'ops.%'`)
	e.DB.Exec(ctx, `UPDATE ops_alerts SET acknowledged_at=now(),last_sent_at=now()-interval '2 days' WHERE code='failures'`)
	if err = e.evaluate(ctx, p, v); err != nil {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='ops.alert.active'`).Scan(&events)
	if events != 1 {
		t.Fatal("ack ignored")
	}
	v["failedJobs10m"] = 0
	e.evaluate(ctx, p, v)
	e.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='ops.alert.recovered'`).Scan(&events)
	if events != 1 {
		t.Fatal("recovery missing")
	}
	delete(v, "diskFreeGB")
	e.evaluate(ctx, p, v)
	var state string
	e.DB.QueryRow(ctx, `SELECT state FROM ops_alerts WHERE code='disk'`).Scan(&state)
	if state != "unknown" {
		t.Fatal("missing sample treated as healthy", state)
	}
	release := Release{Revision: strings.Repeat("b", 40), Image: "dingding229/mediatool:sha-" + strings.Repeat("b", 40), Schema: buildinfo.Schema, Digest: "sha256:" + strings.Repeat("c", 64)}
	if err = e.RegisterRelease(ctx, release); err != nil {
		t.Fatal(err)
	}
	req, err := e.Preview(ctx, "rollback", release.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Approve(ctx, req.ID, req.Digest, "wrong"); err == nil {
		t.Fatal("bad phrase")
	}
	if err = e.Approve(ctx, req.ID, req.Digest, ConfirmText(req)); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Claim(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Claim(ctx, req.ID); err == nil {
		t.Fatal("replayed claimed maintenance")
	}
	if err = e.Resume(ctx, 2); err == nil {
		t.Fatal("resume while executing")
	}
	e.Finish(ctx, req.ID, "fixture only", false)
	if err = e.Resume(ctx, 2); err != nil {
		t.Fatal(err)
	}
	release.Revision = strings.Repeat("d", 40)
	release.Image = "dingding229/mediatool:sha-" + release.Revision
	release.Schema = 49
	e.RegisterRelease(ctx, release)
	if _, err = e.Preview(ctx, "rollback", release.Revision); err == nil {
		t.Fatal("schema downgrade allowed")
	}
	one, queueErr := e.QueueBackup(ctx, "cron:1:owned-slot")
	if queueErr != nil {
		t.Fatal(queueErr)
	}
	e.DB.Exec(ctx, `UPDATE ops_backups SET state='failed'WHERE id=$1`, one)
	two, queueErr := e.QueueBackup(ctx, "cron:1:owned-slot")
	if queueErr != nil || one != two {
		t.Fatal("cron slot repeated", one, two, queueErr)
	}
	e.DB.Exec(ctx, `DELETE FROM ops_backups WHERE id=$1`, one)
	if _, err = e.History(ctx, 0); err == nil {
		t.Fatal("unbounded trends")
	}
	if err = e.Sample(ctx, p); err != nil {
		t.Fatal(err)
	}
	history, err := e.History(ctx, 24)
	if err != nil || len(history) != 1 {
		t.Fatal(history, err)
	}
	for i := 0; i < 4; i++ {
		id := newID()
		e.DB.Exec(ctx, `INSERT INTO ops_backups(id,state,created_at)VALUES($1,'ready',now()-make_interval(days=>$2))`, id, i+1)
		dir, _ := e.dir(id)
		os.Mkdir(dir, 0700)
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("owned"), 0600)
	}
	rows, _ := e.Backups(ctx)
	if err = e.Delete(ctx, rows[0].ID); err == nil {
		t.Fatal("latest backup deleted")
	}
	if err = e.Pin(ctx, rows[3].ID, true); err != nil {
		t.Fatal(err)
	}
	p.KeepCount = 2
	p.KeepDays = 1
	if err = e.Retain(ctx, p); err != nil {
		t.Fatal(err)
	}
	rows, _ = e.Backups(ctx)
	if rows[2].State != "deleted" || rows[3].State != "ready" {
		t.Fatal(rows)
	}
}
func postgresRunner(t *testing.T) Command {
	return func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		t.Helper()
		container := os.Getenv("TEST_POSTGRES_CONTAINER")
		if container == "" {
			container = "media-tool-postgres-1"
		}
		docker, err := exec.LookPath("docker")
		if err != nil {
			return err
		}
		argv := []string{"exec", "-i"}
		for _, v := range env {
			if strings.HasPrefix(v, "PG") && !strings.HasPrefix(v, "PGPORT=") {
				argv = append(argv, "-e", v)
			}
		}
		argv = append(argv, "-e", "PGPORT=5432", "-e", "PGHOST=127.0.0.1", container, name)
		argv = append(argv, args...)
		cmd := exec.CommandContext(ctx, docker, argv...)
		cmd.Stdin = in
		cmd.Stdout = out
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err = cmd.Run(); err != nil {
			t.Log(stderr.String())
			return err
		}
		return nil
	}
}
func TestRealPostgresBackupDrillAndPermanentKeyProof(t *testing.T) {
	e := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("self restore requires shared test-lock")
	}
	cfg, err := pgx.ParseConfig(e.DSN)
	if err != nil || cfg.Database != "media_tool_test" {
		t.Fatal("only media_tool_test")
	}
	e.Run = postgresRunner(t)
	if _, err = e.DB.Exec(ctx, `TRUNCATE integrations,storage_connections,cloud_files,media_assets,media_items,jobs,outbox_events,external_api_keys CASCADE`); err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New(e.Key)
	if err != nil {
		t.Fatal(err)
	}
	key := newID()
	defer func() {
		_, e := e.DB.Exec(context.Background(), `DELETE FROM external_api_keys WHERE id=$1;`, key)
		if e != nil {
			t.Error(e)
		}
	}()
	cipher, err := vault.Seal("api-key:"+key, map[string]string{"token": "synthetic-permanent-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `INSERT INTO external_api_keys(id,name,token_digest,token_ciphertext,prefix,scopes)VALUES($1,'Self restore proof',decode(repeat('ab',32),'hex'),$2,'synthetic','["status.read"]')`, key, cipher); err != nil {
		t.Fatal(err)
	}
	strm := []byte("http://127.0.0.1/api/v1/cloudStorage/strm302?apiKey=fixture-only&path=%2FSynthetic%2Fmovie.mkv\n")
	if _, err = e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES(md5('self-store')::uuid,'Synthetic','clouddrive2');INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES(md5('self-file')::uuid,md5('self-store')::uuid,'/Synthetic/movie.mkv',1);INSERT INTO media_assets(id,cloud_file_id)VALUES(md5('self-asset')::uuid,md5('self-file')::uuid);INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES(md5('self-strm')::uuid,md5('self-asset')::uuid,'Synthetic.strm','fixture-only')`); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(e.OutputRoot, "Synthetic.strm"), strm, 0600); err != nil {
		t.Fatal(err)
	}
	id, err := e.QueueBackup(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, id); err != nil {
		t.Fatal(err)
	}
	m, dir, err := e.VerifyBundle(ctx, id)
	if err != nil || m.Schema != buildinfo.Schema || m.ID != id {
		t.Fatal(m, err)
	}
	info, err := os.Stat(filepath.Join(dir, "database.dump"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup not private", err)
	}
	// No CREATE DATABASE, renamed DB, or exception: restore the synthetic dump
	// back into the same isolated database while the global runner holds its lock.
	if _, err = e.DB.Exec(ctx, `DO $$ BEGIN IF current_database()<>'media_tool_test' THEN RAISE EXCEPTION 'test DB only'; END IF; END $$;DROP SCHEMA public CASCADE;CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	e.DB.Reset()
	dump, err := os.Open(filepath.Join(dir, "database.dump"))
	if err != nil {
		t.Fatal(err)
	}
	restoreErr := e.pg(ctx, "pg_restore", e.DSN, []string{"--clean", "--if-exists", "--exit-on-error", "--no-owner", "--no-acl", "--dbname=media_tool_test"}, dump, io.Discard)
	closeErr := dump.Close()
	if restoreErr != nil || closeErr != nil {
		t.Fatal(restoreErr, closeErr)
	}
	e.DB.Reset()
	report, err := e.verifyRestored(ctx, e.DSN, "media_tool_test", m, true)
	if err != nil || !report.DryRunCiphertextsUnchanged || !report.SecondMigrationIdempotent || !report.NoWorkersStarted {
		t.Fatal(report, err)
	}
	var after []byte
	var permanent bool
	if err = e.DB.QueryRow(ctx, `SELECT token_ciphertext,expires_at IS NULL FROM external_api_keys WHERE id=$1`, key).Scan(&after, &permanent); err != nil || string(after) != string(cipher) || !permanent {
		t.Fatal("permanent key changed", err)
	}
	decoded, err := vault.Open("api-key:"+key, after)
	if err != nil || decoded["token"] != "synthetic-permanent-token" {
		t.Fatal("AAD changed", err)
	}
	if err = os.Remove(filepath.Join(e.OutputRoot, "Synthetic.strm")); err != nil {
		t.Fatal(err)
	}
	if err = e.restoreOutputs(ctx, filepath.Join(dir, "outputs.tar"), m.Files); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(e.OutputRoot, "Synthetic.strm"))
	if err != nil || string(restored) != string(strm) {
		t.Fatal("STRM body changed", err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_backups SET state='ready',digest=$2,manifest=$3 WHERE id=$1`, id, hash(manifestRaw), manifestRaw); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.VerifyBundle(ctx, id); err != nil {
		t.Fatal("restored fixture metadata invalid before corruption", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "outputs.tar"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.VerifyBundle(ctx, id); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Schema names exercise transactional swap/retention without a forbidden
// additional database. The owner's DATABASE SQL plan has separate contract tests.
func TestOfflineSchemaSwapIsAtomicAndRetainsOriginalFixture(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	a := "q5a_" + stripID(newID())
	b := "q5b_" + stripID(newID())
	old := "q5old_" + stripID(newID())
	for _, name := range []string{a, b} {
		if _, err := e.DB.Exec(ctx, "CREATE SCHEMA "+quote(name)); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, name := range []string{a, b, old} {
			if _, err := e.DB.Exec(ctx, "DROP SCHEMA IF EXISTS "+quote(name)+" CASCADE"); err != nil {
				t.Error(err)
			}
		}
	}()
	if _, err := e.DB.Exec(ctx, "CREATE TABLE "+quote(a)+".proof(value int);INSERT INTO "+quote(a)+".proof VALUES(71)"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(ctx, "BEGIN;ALTER SCHEMA "+quote(a)+" RENAME TO "+quote(old)+";ALTER SCHEMA "+quote(b)+" RENAME TO "+quote(a)+";COMMIT;"); err != nil {
		t.Fatal(err)
	}
	var proof int
	if err := e.DB.QueryRow(ctx, "SELECT value FROM "+quote(old)+".proof").Scan(&proof); err != nil || proof != 71 {
		t.Fatal("original fixture was not retained", err)
	}
}
