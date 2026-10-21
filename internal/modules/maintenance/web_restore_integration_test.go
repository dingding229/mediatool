package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/platform/buildinfo"
)

// No extra database is created by these tests. Candidate validation is exercised
// by the existing real self-restore/verifier test; here a preparation seam builds
// synthetic candidates in media_tool_test, then the real transactional pg_restore
// and the production state machine run unchanged.
func webFixture(t *testing.T) (Engine, string) {
	t.Helper()
	e := fixture(t)
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("test-lock required")
	}
	basePG := postgresRunner(t)
	e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		args = append([]string{}, args...)
		for i, arg := range args {
			if strings.HasPrefix(arg, "--use-list=") {
				path := strings.TrimPrefix(arg, "--use-list=")
				remote := "/tmp/media-tool-b4-" + newID() + ".list"
				container := os.Getenv("TEST_POSTGRES_CONTAINER")
				if container == "" {
					container = "media-tool-postgres-1"
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-c", `umask 077;cat > "$1"`, "sh", remote)
				cmd.Stdin = strings.NewReader(string(raw))
				if err = cmd.Run(); err != nil {
					return err
				}
				defer exec.Command("docker", "exec", container, "rm", "-f", remote).Run()
				args[i] = "--use-list=" + remote
			}
		}
		// Reordered custom-archive blocks require a seekable file, just as
		// production exec.Cmd receives the *os.File directly (not Docker stdin).
		if name == "pg_restore" && strings.Contains(strings.Join(args, " "), "--use-list=") {
			remote := "/tmp/media-tool-b4b-" + newID() + ".dump"
			container := os.Getenv("TEST_POSTGRES_CONTAINER")
			if container == "" {
				container = "media-tool-postgres-1"
			}
			copy := exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-c", `umask 077;cat > "$1"`, "sh", remote)
			copy.Stdin = in
			if err := copy.Run(); err != nil {
				return err
			}
			defer exec.Command("docker", "exec", container, "rm", "-f", remote).Run()
			args = append(args, remote)
			in = nil
		}
		return basePG(ctx, name, args, env, in, out)
	}
	e.DB.Exec(context.Background(), `DELETE FROM audit_logs WHERE action='b4.live'`)
	t.Cleanup(func() { e.DB.Exec(context.Background(), `DELETE FROM audit_logs WHERE action='b4.live'`) })
	ctx := context.Background()
	if _, err := e.DB.Exec(ctx, `TRUNCATE integrations,storage_connections,cloud_files,media_assets,media_items,jobs,outbox_events,external_api_keys CASCADE`); err != nil {
		t.Fatal(err)
	}
	e.validateRestore = func(ctx context.Context, m Manifest, dir string) error {
		// Production uses the same bundle/key/schema checks and a clone verifier.
		return nil
	}
	target := newID()
	if _, err := e.DB.Exec(ctx, `INSERT INTO ops_backups(id,origin)VALUES($1,'web-test')`, target); err != nil {
		t.Fatal(err)
	}
	if err := e.Backup(ctx, target); err != nil {
		t.Fatal(err)
	}
	e.prepareRestore = func(ctx context.Context, r WebRestore, m Manifest, dir string) (string, func() error, error) {
		// Build a current-catalog candidate without a second test database. Only the
		// synthetic marker stands in for business contents. The actual import is real.
		var prior *string
		if err := e.DB.QueryRow(ctx, `SELECT(SELECT backup_id::text FROM ops_restore_commit WHERE request_id=$1)`, r.ID).Scan(&prior); err != nil {
			return "", nil, err
		}
		if _, err := e.DB.Exec(ctx, `INSERT INTO ops_restore_commit(request_id,backup_id)VALUES($1,$2)ON CONFLICT(request_id)DO UPDATE SET backup_id=excluded.backup_id`, r.ID, m.ID); err != nil {
			return "", nil, err
		}
		var previous int
		e.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='b4.live'`).Scan(&previous)
		if _, err := e.DB.Exec(ctx, `DELETE FROM audit_logs WHERE action='b4.live'`); err != nil {
			return "", nil, err
		}
		if m.ID != target {
			if _, err := e.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('test','b4.live','{}')`); err != nil {
				return "", nil, err
			}
		}
		f, err := os.CreateTemp(e.Root, "test-candidate-*.dump")
		if err != nil {
			return "", nil, err
		}
		err = e.pg(ctx, "pg_dump", e.DSN, []string{"--format=custom", "--no-owner", "--no-acl"}, nil, f)
		err = errors.Join(err, f.Sync(), f.Close())
		_, restoreErr := e.DB.Exec(ctx, `DELETE FROM ops_restore_commit WHERE request_id=$1`, r.ID)
		if prior != nil {
			_, restoreErr = e.DB.Exec(ctx, `INSERT INTO ops_restore_commit(request_id,backup_id)VALUES($1,$2)ON CONFLICT(request_id)DO UPDATE SET backup_id=excluded.backup_id`, r.ID, *prior)
		}
		_, restoreAuditErr := e.DB.Exec(ctx, `DELETE FROM audit_logs WHERE action='b4.live'`)
		if previous > 0 {
			_, restoreAuditErr = e.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('test','b4.live','{}')`)
		}
		restoreErr = errors.Join(restoreErr, restoreAuditErr)
		return f.Name(), func() error { return os.Remove(f.Name()) }, errors.Join(err, restoreErr)
	}
	return e, target
}
func queueWeb(t *testing.T, e Engine, target string) WebRestore {
	t.Helper()
	ctx := context.Background()
	r, err := e.PreviewWebRestore(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target)
	if err != nil || r.State != "pending" {
		t.Fatal(r, err)
	}
	return r
}
func TestWebRestoreSuccessRollbackAndIdempotentAdmission(t *testing.T) {
	e, target := webFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	other, err := e.PreviewWebRestore(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	r := queueWeb(t, e, target)
	same, err := e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target)
	if err != nil || same.ID != r.ID {
		t.Fatal(same, err)
	}
	if _, err = e.QueueBackup(ctx, "manual"); !errors.Is(err, ErrConflict) {
		t.Fatal("backup admitted during restore", err)
	}
	if _, err = e.QueueDrill(ctx, target); !errors.Is(err, ErrConflict) {
		t.Fatal("drill admitted during restore", err)
	}
	if _, err = e.QueueWebRestore(ctx, other.ID, other.Digest, "恢复 "+target); !errors.Is(err, ErrConflict) {
		t.Fatal("concurrent restore admitted", err)
	}
	if _, err = e.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('test','b4.live','{}')`); err != nil {
		t.Fatal(err)
	}
	if err = e.RunWebRestore(ctx); err != nil {
		t.Fatal(err)
	}
	e.DB.Reset() // Fresh test connections must see the commit; live table OIDs are retained.
	got, err := e.WebRestore(ctx, r.ID)
	if err != nil || got.State != "completed" || got.SnapshotID == nil || got.AppliedBackup == nil || *got.AppliedBackup != target {
		t.Fatal(got, err)
	}
	var live int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='b4.live'`).Scan(&live)
	if live != 0 {
		t.Fatal("candidate not restored")
	}
	if err = e.Pin(ctx, *got.SnapshotID, false); !errors.Is(err, ErrConflict) {
		t.Fatal("latest rollback snapshot unpinned", err)
	}
	if err = e.Delete(ctx, *got.SnapshotID); err == nil {
		t.Fatal("latest rollback snapshot removed")
	}
	again, err := e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target)
	if err != nil || again.State != "completed" {
		t.Fatal(again, err)
	}
	// Same preview/confirmation path is used for rollback-to-preflight.
	rollback := queueWeb(t, e, *got.SnapshotID)
	if err = e.RunWebRestore(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := e.WebRestore(ctx, rollback.ID)
	if err != nil || after.State != "completed" {
		t.Fatal(after, err)
	}
	e.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='b4.live'`).Scan(&live)
	if live != 1 {
		t.Fatal("rollback did not restore preflight contents", live)
	}
	paused, err := Paused(ctx, e.DB)
	if err != nil || paused {
		t.Fatal("maintenance not released", err)
	}
}
func TestWebRestoreChecksumSchemaKeyRejectedAndChangedBusinessAllowed(t *testing.T) {
	e, target := webFixture(t)
	ctx := context.Background()
	m, dir, err := e.VerifyBundle(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	rewrite := func(m Manifest) {
		t.Helper()
		raw, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.DB.Exec(ctx, `UPDATE ops_backups SET digest=$2,manifest=$3 WHERE id=$1`, target, hash(raw), raw); err != nil {
			t.Fatal(err)
		}
	}
	m.Schema = buildinfo.Schema + 1
	rewrite(m)
	if _, err = e.PreviewWebRestore(ctx, target); err == nil || !strings.Contains(err.Error(), "高于") {
		t.Fatal(err)
	}
	m.Schema = buildinfo.Schema
	m.KeyFingerprint = "bad"
	rewrite(m)
	if _, err = e.PreviewWebRestore(ctx, target); err == nil {
		t.Fatal("mismatched key admitted")
	}
	m.KeyFingerprint = hash([]byte("key:" + e.Key))
	rewrite(m)
	if _, err = e.DB.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'tmdb','changed','fixture')`, newID()); err != nil {
		t.Fatal(err)
	}
	if _, err = e.PreviewWebRestore(ctx, target); err != nil {
		t.Fatal("changed credentials must be allowed", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "database.dump"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = e.PreviewWebRestore(ctx, target); err == nil {
		t.Fatal("corrupt dump admitted")
	}
}
func TestWebRestorePreflightFailureAbortsAndImportFailureRollsBack(t *testing.T) {
	for _, fail := range []string{"snapshot", "import"} {
		t.Run(fail, func(t *testing.T) {
			e, target := webFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			r := queueWeb(t, e, target)
			_, err := e.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('test','b4.live','{}')`)
			if err != nil {
				t.Fatal(err)
			}
			base := e.Run
			imports := 0
			e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
				if fail == "snapshot" && name == "pg_dump" {
					return errors.New("injected snapshot failure")
				}
				if name == "psql" {
					imports++
					if imports == 1 {
						if err := base(ctx, name, args, env, in, out); err != nil {
							return err
						}
						return errors.New("injected acknowledgement loss after import committed")
					}
				}
				return base(ctx, name, args, env, in, out)
			}
			if err := e.RunWebRestore(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := e.WebRestore(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if fail == "import" {
				want = "rolled_back"
				if imports != 2 {
					t.Fatal(imports)
				}
			} else if imports != 0 {
				t.Fatal("restored after snapshot failure")
			}
			var marker int
			if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='b4.live'`).Scan(&marker); err != nil || marker != 1 {
				t.Fatal("preflight content not restored", marker, err)
			}
			if got.State != want {
				t.Fatal(got)
			}
			paused, err := Paused(ctx, e.DB)
			if err != nil || paused {
				t.Fatal(paused, err)
			}
		})
	}
}
func TestWebRestoreConcurrentQueueAndInterruptedReconciliation(t *testing.T) {
	e, target := webFixture(t)
	ctx := context.Background()
	r, err := e.PreviewWebRestore(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { _, err := e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM ops_web_restores WHERE state='pending'`).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	_, err = e.DB.Exec(ctx, `UPDATE ops_web_restores SET state='running' WHERE id=$1;`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.DB.Exec(ctx, `UPDATE ops_mode SET paused=true,reason=$1 WHERE singleton`, "网页恢复 "+r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.RunWebRestore(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := e.WebRestore(ctx, r.ID)
	if err != nil || got.State != "unknown" {
		t.Fatal(got, err)
	}
	if _, err = e.QueueBackup(ctx, "manual"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	got, err = e.ReconcileWebRestore(ctx, r.ID)
	if err != nil || got.State != "failed" {
		t.Fatal(got, err)
	}
}

func TestRestoreRetainsOnlyCoordinationNotBusiness(t *testing.T) {
	for _, name := range []string{"administrator_accounts", "administrator_sessions", "integrations", "open115_authorizations", "application_settings", "plugin_v2_configs", "plugin_v2_kv", "ops_policy", "ops_alerts", "ops_samples"} {
		if retainedRestoreTable(name) {
			t.Fatal("business table retained", name)
		}
	}
	for _, name := range []string{"ops_backups", "ops_web_restores", "ops_restore_commit", "ops_mode"} {
		if !retainedRestoreTable(name) {
			t.Fatal("coordination table lost", name)
		}
	}
}

func TestMaintenanceGateDrainsInFlightWork(t *testing.T) {
	e := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := BeginWork(ctx, e.DB)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		release()
		t.Fatal(err)
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220883)`).Scan(&locked); err != nil || locked {
		release()
		t.Fatal("writer not fenced", locked, err)
	}
	release()
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220883)`).Scan(&locked); err != nil || !locked {
		t.Fatal("writer not drained", err)
	}
	if _, err = BeginWork(ctx, e.DB); !errors.Is(err, ErrConflict) {
		t.Fatal("new work entered exclusive gate", err)
	}
	conn.Exec(ctx, `SELECT pg_advisory_unlock(91220883)`)
	e.DB.Exec(ctx, `UPDATE ops_mode SET paused=true WHERE singleton`)
	if _, err = BeginWork(ctx, e.DB); !errors.Is(err, ErrConflict) {
		t.Fatal("new work entered paused maintenance", err)
	}
}

func TestWebRestoreStatusAndAuthReadableWhileImportWaitsOnBusinessLock(t *testing.T) {
	e, target := webFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := queueWeb(t, e, target)
	blocker, err := e.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(ctx, `LOCK TABLE administrator_accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- e.RunWebRestore(ctx) }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var waiting bool
		if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name='mediatool-maintenance' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("import never waited", err)
		case <-ctx.Done():
			t.Fatal("import lock timeout")
		case <-tick.C:
		}
	}
	read, c := context.WithTimeout(ctx, time.Second)
	defer c()
	records, err := e.WebRestores(read)
	if err != nil || len(records) == 0 {
		t.Fatal("progress blocked by import", err)
	}
	var sessions int
	if err = e.DB.QueryRow(read, `SELECT count(*) FROM administrator_sessions s JOIN administrator_accounts a ON a.id=s.account_id`).Scan(&sessions); err != nil {
		t.Fatal("auth blocked by import", err)
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	got, err := e.WebRestore(ctx, r.ID)
	if err != nil || got.State != "completed" {
		t.Fatal(got, err)
	}
}

func TestWebRestoreFailedCompensationStaysUnknownAndReconcilesStamp(t *testing.T) {
	e, target := webFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := e.PreviewWebRestore(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.QueueWebRestore(ctx, r.ID, r.Digest, "wrong"); !errors.Is(err, ErrUnsafe) {
		t.Fatal("confirmation bypass", err)
	}
	if _, err = e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+target); err != nil {
		t.Fatal(err)
	}
	base := e.Run
	imports := 0
	e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		if name == "psql" {
			imports++
			if imports == 1 {
				if err := base(ctx, name, args, env, in, out); err != nil {
					return err
				}
				return errors.New("lost commit receipt")
			}
			return errors.New("injected rollback failure")
		}
		return base(ctx, name, args, env, in, out)
	}
	if err = e.RunWebRestore(ctx); err == nil {
		t.Fatal("compensation failure hidden")
	}
	got, err := e.WebRestore(ctx, r.ID)
	if err != nil || got.State != "unknown" {
		t.Fatal(got, err)
	}
	paused, err := Paused(ctx, e.DB)
	if err != nil || !paused {
		t.Fatal("unknown resumed writers", err)
	}
	got, err = e.ReconcileWebRestore(ctx, r.ID)
	if err != nil || got.State != "completed" {
		t.Fatal(got, err)
	}
	if imports != 2 {
		t.Fatal("reconciliation replayed an import", imports)
	}
}
