package maintenance

import (
	"context"
	"media-tool/internal/modules/jobs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupDeleteCommitsIntentBeforeRemovingFilesAndCanResume(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	ids := []string{jobs.UUID(), jobs.UUID(), jobs.UUID()}
	for i, id := range ids {
		if _, err := e.DB.Exec(ctx, `INSERT INTO ops_backups(id,state,created_at)VALUES($1,'ready',now()-make_interval(days=>$2))`, id, 3-i); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := e.dir(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"database.dump", "outputs.tar", "manifest.json"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte("owned-backup"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const drop = `DROP TRIGGER IF EXISTS owned_delete_failure ON ops_backups;DROP FUNCTION IF EXISTS owned_delete_failure();`
	defer e.DB.Exec(ctx, drop)
	if _, err = e.DB.Exec(ctx, `CREATE FUNCTION owned_delete_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='deleting' THEN RAISE EXCEPTION 'owned intent failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER owned_delete_failure BEFORE UPDATE ON ops_backups FOR EACH ROW EXECUTE FUNCTION owned_delete_failure()`); err != nil {
		t.Fatal(err)
	}
	if err = e.Delete(ctx, ids[0]); err == nil {
		t.Fatal("intent failure ignored")
	}
	for _, name := range []string{"database.dump", "outputs.tar", "manifest.json"} {
		b, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil || string(b) != "owned-backup" {
			t.Fatal("uncommitted intent deleted backup", name, readErr)
		}
	}
	if _, err = e.DB.Exec(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `CREATE FUNCTION owned_delete_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='deleted' THEN RAISE EXCEPTION 'owned completion failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER owned_delete_failure BEFORE UPDATE ON ops_backups FOR EACH ROW EXECUTE FUNCTION owned_delete_failure()`); err != nil {
		t.Fatal(err)
	}
	if err = e.Delete(ctx, ids[0]); err == nil {
		t.Fatal("completion failure ignored")
	}
	var state string
	if err = e.DB.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, ids[0]).Scan(&state); err != nil || state != "deleting" {
		t.Fatal("incomplete delete advertised ready", state, err)
	}
	if _, err = e.DB.Exec(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if err = e.Delete(ctx, ids[0]); err == nil {
		t.Fatal("active deleting intent accepted another deletion")
	}
	if err = e.Retain(ctx, Defaults()); err != nil {
		t.Fatal(err)
	}
	if err = e.DB.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, ids[0]).Scan(&state); err != nil || state != "deleting" {
		t.Fatal("fresh intent recovered too early", state, err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_backups SET deletion_started_at=now()-interval '11 minutes' WHERE id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err = e.Retain(ctx, Defaults()); err != nil {
		t.Fatal("stale completion could not recover", err)
	}
	if err = e.DB.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, ids[0]).Scan(&state); err != nil || state != "deleted" {
		t.Fatal(state, err)
	}
}

func TestExpiredBackupDeletionIsRecoverableBeforeAndAfterIsolation(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name                string
		original, tombstone bool
	}{
		{"before isolation", true, false},
		{"after isolation", false, true},
		{"both paths remain", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := jobs.UUID()
			oldAttempt := time.Now().Add(-11 * time.Minute).Truncate(time.Microsecond)
			if _, err := e.DB.Exec(ctx, `INSERT INTO ops_backups(id,state,deletion_started_at,created_at)VALUES($1,'deleting',$2,now()-interval '100 days')`, id, oldAttempt); err != nil {
				t.Fatal(err)
			}
			dir, err := e.dir(id)
			if err != nil {
				t.Fatal(err)
			}
			for path, exists := range map[string]bool{dir: tc.original, dir + ".deleting": tc.tombstone} {
				if exists {
					if err = os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(path, "manifest.json"), []byte("owned backup"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = e.Retain(ctx, Defaults()); err != nil {
				t.Fatal(err)
			}
			var state, message string
			wantMessage := "可重新删除"
			if tc.original && tc.tombstone {
				wantMessage = "需人工核对"
			}
			if err = e.DB.QueryRow(ctx, `SELECT state,message FROM ops_backups WHERE id=$1`, id).Scan(&state, &message); err != nil || state != "failed" || !strings.Contains(message, wantMessage) {
				t.Fatal("stale intent not exposed for retry", state, message, err)
			}
			for path, exists := range map[string]bool{dir: tc.original, dir + ".deleting": tc.tombstone} {
				if exists {
					raw, readErr := os.ReadFile(filepath.Join(path, "manifest.json"))
					if readErr != nil || string(raw) != "owned backup" {
						t.Fatal("recovery changed backup files", path, readErr)
					}
				}
			}
			if tc.original && tc.tombstone {
				if err = e.Delete(ctx, id); err == nil {
					t.Fatal("conflicting isolation directory overwritten")
				}
				return
			}
			if err = e.Delete(ctx, id); err != nil {
				t.Fatal("recovered backup could not be deleted", err)
			}
			if err = e.DB.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, id).Scan(&state); err != nil || state != "deleted" {
				t.Fatal(state, err)
			}
			for _, path := range []string{dir, dir + ".deleting"} {
				if _, err = os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("backup path retained", path, err)
				}
			}
			// An old attempt callback must not replace the completed retry.
			_ = e.failBackupDeletion(ctx, id, oldAttempt, "stale: ", context.DeadlineExceeded)
			if err = e.DB.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, id).Scan(&state); err != nil || state != "deleted" {
				t.Fatal("old attempt overwrote retry", state, err)
			}
		})
	}
}
