package maintenance

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/database"
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
	e.Run = postgresRunner(t)
	// This suite owns only temporary DB/files. Clear unrelated fixture media because
	// they do not represent files in this test's private output root.
	if _, err := e.DB.Exec(ctx, `TRUNCATE integrations,storage_connections,cloud_files,media_assets,media_items,jobs,outbox_events,external_api_keys CASCADE`); err != nil {
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
	if err != nil || m.Schema != 50 || m.ID != id {
		t.Fatal(m, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "database.dump"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup not private")
	}
	drill, err := e.QueueDrill(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Drill(ctx, drill); err != nil {
		t.Fatal(err)
	}
	list, _ := e.Drills(ctx)
	if list[0].State != "verified" {
		t.Fatal(list)
	}
	var leftovers int
	e.DB.QueryRow(ctx, `SELECT count(*)FROM pg_database WHERE datname=$1`, "media_tool_restore_"+stripID(drill)[:12]).Scan(&leftovers)
	if leftovers != 0 {
		t.Fatal("drill database not cleaned")
	}
	req, err := e.Preview(ctx, "restore", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Approve(ctx, req.ID, req.Digest, ConfirmText(req)); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Claim(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	candidate, err := e.PrepareCandidate(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer e.DB.Exec(context.Background(), `DROP DATABASE `+quote(candidate)+` WITH(FORCE)`)
	if !strings.HasPrefix(candidate, "media_tool_restore_") {
		t.Fatal(candidate)
	}
	e.Finish(ctx, req.ID, "isolated offline clone only", false)
	var revision int64
	e.DB.QueryRow(ctx, `SELECT revision FROM ops_mode WHERE singleton`).Scan(&revision)
	e.Resume(ctx, revision)
	// Any archive mutation blocks verify/restore; never trust a DB state label alone.
	os.WriteFile(filepath.Join(dir, "outputs.tar"), []byte("tampered"), 0600)
	if _, _, err = e.VerifyBundle(ctx, id); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	_ = json.Valid
}
func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func TestOfflineDatabaseRenameIsAtomicAndRetainsOldDatabase(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	a := "media_tool_restore_" + stripID(newID())[:12]
	b := "media_tool_restore_" + stripID(newID())[:12]
	old := "media_tool_before_" + stripID(newID())[:12]
	for _, name := range []string{a, b} {
		if _, err := e.DB.Exec(ctx, `CREATE DATABASE `+quote(name)+` TEMPLATE template0`); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, name := range []string{a, b, old} {
			e.DB.Exec(context.Background(), `DROP DATABASE IF EXISTS `+quote(name)+` WITH(FORCE)`)
		}
	}()
	if _, err := e.DB.Exec(ctx, `BEGIN;ALTER DATABASE `+quote(a)+` RENAME TO `+quote(old)+`;ALTER DATABASE `+quote(b)+` RENAME TO `+quote(a)+`;COMMIT;`); err != nil {
		t.Fatal(err)
	}
	var count int
	e.DB.QueryRow(ctx, `SELECT count(*)FROM pg_database WHERE datname IN($1,$2)`, a, old).Scan(&count)
	if count != 2 {
		t.Fatal("original database was not retained")
	}
}
