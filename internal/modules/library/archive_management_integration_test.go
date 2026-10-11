package library

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type archiveFixture struct {
	ctx                                     context.Context
	db                                      *pgxpool.Pool
	e                                       Engine
	root, archive, archivePath, asset, next string
	raw                                     []byte
}

func newArchiveFixture(t *testing.T) archiveFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
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
		t.Fatal("isolated PG required")
	}
	db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	t.Cleanup(func() { db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`) })
	// Approval rows refer to journal records; explicitly clean only these fixture tables.
	db.Exec(ctx, `TRUNCATE media_archive_cleanup_operations,media_archive_cleanup_requests`)
	t.Cleanup(func() { db.Exec(ctx, `TRUNCATE media_archive_cleanup_operations,media_archive_cleanup_requests`) })
	db.Exec(ctx, `UPDATE media_archive_policy SET retention_days=30,revision=1 WHERE singleton`)
	t.Cleanup(func() { db.Exec(ctx, `UPDATE media_archive_policy SET retention_days=30,revision=1 WHERE singleton`) })
	vault, _ := secrets.New(strings.Repeat("a", 64))
	root := t.TempDir()
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{STRMOutputDir: root})
	t.Cleanup(m.Close)
	e := Engine{DB: db, Connections: m}
	j := Journal{DB: db, Vault: vault}
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, storage)
	db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, file, storage)
	db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
	raw := []byte("owned generated fixed URL\n")
	old, next := "Movies/old.strm", "TV/new.strm"
	if err = j.Write(ctx, root, old, asset, "asset:"+asset, "strm", raw); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	write, err := j.Stage(ctx, tx, root, next, asset, "asset:"+asset, "strm", raw)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := j.StageArchive(ctx, tx, root, old, asset, "strm", []string{write})
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	if err = j.Apply(ctx, write); err != nil {
		t.Fatal(err)
	}
	if err = j.Apply(ctx, archive); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'fixture-key-reference')`, jobs.UUID(), asset, next); err != nil {
		t.Fatal(err)
	}
	var archivePath string
	db.QueryRow(ctx, `SELECT archive_relative FROM media_artifact_journal WHERE id=$1`, archive).Scan(&archivePath)
	return archiveFixture{ctx, db, e, root, archive, archivePath, asset, next, raw}
}

func TestArchiveRetentionPreviewConsentIdempotencyAndUserDataPreservation(t *testing.T) {
	f := newArchiveFixture(t)
	ctx, db, e := f.ctx, f.db, f.e
	root, archive, archivePath, next, raw := f.root, f.archive, f.archivePath, f.next, f.raw
	var err error
	policy, err := e.ArchivePolicy(ctx)
	if err != nil || policy.RetentionDays != 30 || policy.AutomaticCleanup {
		t.Fatal("unsafe default policy")
	}
	if _, err = e.PreviewArchiveCleanup(ctx, []string{archive}); err == nil {
		t.Fatal("fresh archive eligible")
	}
	list, total, err := e.Archives(ctx, 50, 0)
	if err != nil || total != 1 || len(list) != 1 || list[0].State != "retained" {
		t.Fatal("archive listing", err)
	}
	policy.RetentionDays = 1
	policy, err = e.SaveArchivePolicy(ctx, policy)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `UPDATE media_artifact_journal SET updated_at=now()-interval '2 days' WHERE id=$1`, archive)
	preview, err := e.PreviewArchiveCleanup(ctx, []string{archive})
	if err != nil || len(preview.Items) != 1 || !preview.PhysicalDeletion || !preview.SourceFilesUntouched {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, archivePath)); err != nil {
		t.Fatal("preview removed archive")
	}
	request := ArchiveCleanupRequest{RequestID: jobs.UUID(), ArchiveIDs: []string{archive}, PlanDigest: preview.PlanDigest}
	wrong := request
	wrong.PlanDigest = strings.Repeat("0", 64)
	if _, err = e.ConfirmArchiveCleanup(ctx, wrong); err == nil {
		t.Fatal("stale/missing proof accepted")
	}
	os.WriteFile(filepath.Join(root, archivePath), []byte("manual"), 0640)
	if _, err = e.ConfirmArchiveCleanup(ctx, request); !errors.Is(err, ErrArtifactConflict) {
		t.Fatal("modified archive accepted", err)
	}
	os.WriteFile(filepath.Join(root, archivePath), raw, 0640)
	policy.RetentionDays = 3
	policy, err = e.SaveArchivePolicy(ctx, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ConfirmArchiveCleanup(ctx, request); err == nil {
		t.Fatal("changed retention policy ignored")
	}
	policy.RetentionDays = 1
	policy, err = e.SaveArchivePolicy(ctx, policy)
	if err != nil {
		t.Fatal(err)
	}
	preview, err = e.PreviewArchiveCleanup(ctx, request.ArchiveIDs)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanDigest = preview.PlanDigest
	result, err := e.ConfirmArchiveCleanup(ctx, request)
	if err != nil || len(result) != 1 || result[0].State != "removed" {
		t.Fatal("explicit cleanup not finished", result, err)
	}
	if _, err = os.Stat(filepath.Join(root, archivePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("approved expired archive not removed")
	}
	active, _ := os.ReadFile(filepath.Join(root, next))
	if string(active) != string(raw) {
		t.Fatal("active output altered")
	}
	result, err = e.ConfirmArchiveCleanup(ctx, request)
	if err != nil || result[0].State != "removed" {
		t.Fatal("request replay changed outcome", err)
	}
	request.PlanDigest = strings.Repeat("f", 64)
	if _, err = e.ConfirmArchiveCleanup(ctx, request); err == nil {
		t.Fatal("request ID reused for different plan")
	}
	list, total, err = e.Archives(ctx, 50, 0)
	if err != nil || total != 1 || list[0].DeletedAt == nil || list[0].State != "removed" {
		t.Fatal("cleanup audit record discarded")
	}
	if _, err = e.PreviewArchiveCleanup(ctx, []string{archive, archive}); err == nil {
		t.Fatal("duplicate selection accepted")
	}
	policy.AutomaticCleanup = true
	if _, err = e.SaveArchivePolicy(ctx, policy); err == nil {
		t.Fatal("automatic deletion accepted")
	}
}

// Creates an approval only, like a request interrupted immediately after its
// durable commit. It deliberately does not call the destructive executor.
func (f archiveFixture) approve(t *testing.T) (ArchiveCleanupRequest, string, string) {
	t.Helper()
	f.exec(t, `UPDATE media_archive_policy SET retention_days=1,revision=revision+1 WHERE singleton`)
	f.exec(t, `UPDATE media_artifact_journal SET updated_at=now()-interval '2 days' WHERE id=$1`, f.archive)
	preview, err := f.e.PreviewArchiveCleanup(f.ctx, []string{f.archive})
	if err != nil {
		t.Fatal(err)
	}
	request := ArchiveCleanupRequest{jobs.UUID(), []string{f.archive}, preview.PlanDigest}
	id := jobs.UUID()
	quarantine := filepath.ToSlash(filepath.Join(filepath.Dir(f.archivePath), ".cleanup-"+id+".retired"))
	f.exec(t, `INSERT INTO media_archive_cleanup_requests(id,plan_digest,archive_ids,policy_revision,retention_days) VALUES($1,$2,$3::uuid[],$4,$5)`, request.RequestID, request.PlanDigest, request.ArchiveIDs, preview.Policy.Revision, preview.Policy.RetentionDays)
	f.exec(t, `INSERT INTO media_archive_cleanup_operations(id,request_id,journal_id,expected_digest,quarantine_relative) SELECT $1,$2,id,expected_digest,$4 FROM media_artifact_journal WHERE id=$3`, id, request.RequestID, f.archive, quarantine)
	return request, id, quarantine
}
func (f archiveFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}
func (f archiveFixture) operation(t *testing.T, id, want string) {
	t.Helper()
	var state string
	if err := f.db.QueryRow(f.ctx, `SELECT state FROM media_archive_cleanup_operations WHERE id=$1`, id).Scan(&state); err != nil || state != want {
		t.Fatal("unexpected durable phase", state, want, err)
	}
}
func (f archiveFixture) preserved(t *testing.T, relative string) {
	t.Helper()
	actual, err := os.ReadFile(filepath.Join(f.root, relative))
	if err != nil || string(actual) != string(f.raw) {
		t.Fatal("owned file lost/changed", relative, err)
	}
}
func TestArchiveCleanupDurablePhasesNeedExplicitRetry(t *testing.T) {
	for _, gap := range []string{"approval-commit", "rename-before-quarantine-commit", "unlink-before-removal-commit", "missing-before-durable-quarantine"} {
		t.Run(gap, func(t *testing.T) {
			f := newArchiveFixture(t)
			request, id, quarantine := f.approve(t)
			sum := sha256.Sum256(f.raw)
			want := "approved"
			switch gap {
			case "rename-before-quarantine-commit", "unlink-before-removal-commit":
				if ok, err := quarantineArchive(f.root, f.archivePath, quarantine, sum[:]); !ok || err != nil {
					t.Fatal("quarantine fixture", err)
				}
				if gap == "unlink-before-removal-commit" {
					want = "quarantined"
					f.exec(t, `UPDATE media_archive_cleanup_operations SET state='quarantined' WHERE id=$1`, id)
					if ok, err := removeQuarantined(f.root, f.archivePath, quarantine, sum[:]); !ok || err != nil {
						t.Fatal("unlink fixture", err)
					}
				}
			case "missing-before-durable-quarantine":
				if err := os.Remove(filepath.Join(f.root, f.archivePath)); err != nil {
					t.Fatal(err)
				}
			}
			// Case variants are the same UUID and must not create another approval.
			request.RequestID = strings.ToUpper(request.RequestID)
			request.ArchiveIDs[0] = strings.ToUpper(request.ArchiveIDs[0])
			result, err := f.e.ConfirmArchiveCleanup(f.ctx, request)
			if err != nil || len(result) != 1 || result[0].State != want {
				t.Fatal("confirmation replay must only observe", result, err)
			}
			f.operation(t, id, want)
			if gap == "approval-commit" {
				f.preserved(t, f.archivePath)
			} else if gap == "rename-before-quarantine-commit" {
				f.preserved(t, quarantine)
			}
			err = f.e.ApplyArchiveCleanup(f.ctx, id)
			if gap == "missing-before-durable-quarantine" {
				if !errors.Is(err, ErrArtifactConflict) {
					t.Fatal("unknown disappearance treated as removal", err)
				}
				f.operation(t, id, "conflict")
			} else {
				if err != nil {
					t.Fatal("explicit retry did not recover", err)
				}
				f.operation(t, id, "removed")
				if err = f.e.ApplyArchiveCleanup(f.ctx, id); err != nil {
					t.Fatal("completed retry not idempotent", err)
				}
			}
			f.preserved(t, f.next)
		})
	}
}
func TestArchiveCleanupRetryRechecksCurrentOutputPolicyAndRecoveryState(t *testing.T) {
	for _, changed := range []string{"output-missing", "output-modified", "retention-increased", "pending-asset", "pending-journal", "quarantined-retention-increased"} {
		t.Run(changed, func(t *testing.T) {
			f := newArchiveFixture(t)
			_, id, quarantine := f.approve(t)
			retained := f.archivePath
			switch changed {
			case "output-missing":
				os.Remove(filepath.Join(f.root, f.next))
			case "output-modified":
				os.WriteFile(filepath.Join(f.root, f.next), []byte("manual"), 0640)
			case "quarantined-retention-increased":
				sum := sha256.Sum256(f.raw)
				if ok, err := quarantineArchive(f.root, f.archivePath, quarantine, sum[:]); !ok || err != nil {
					t.Fatal(err)
				}
				f.exec(t, `UPDATE media_archive_cleanup_operations SET state='quarantined' WHERE id=$1`, id)
				retained = quarantine
				fallthrough
			case "retention-increased":
				f.exec(t, `UPDATE media_archive_policy SET retention_days=30,revision=revision+1 WHERE singleton`)
			case "pending-asset":
				f.exec(t, `UPDATE media_assets SET artifacts_state='pending' WHERE id=$1`, f.asset)
			case "pending-journal":
				f.exec(t, `UPDATE media_artifact_journal SET state='pending' WHERE asset_id=$1 AND operation='write'`, f.asset)
			}
			if err := f.e.ApplyArchiveCleanup(f.ctx, id); !errors.Is(err, ErrArtifactConflict) {
				t.Fatal("stale approval bypassed safety checks", changed, err)
			}
			f.operation(t, id, "conflict")
			f.preserved(t, retained)
			var deleted *time.Time
			if err := f.db.QueryRow(f.ctx, `SELECT archive_deleted_at FROM media_artifact_journal WHERE id=$1`, f.archive).Scan(&deleted); err != nil || deleted != nil {
				t.Fatal("conflict stamped as removed", err)
			}
			os.WriteFile(filepath.Join(f.root, f.next), f.raw, 0640)
			f.exec(t, `UPDATE media_archive_policy SET retention_days=1,revision=revision+1 WHERE singleton`)
			f.exec(t, `UPDATE media_assets SET artifacts_state='ready' WHERE id=$1`, f.asset)
			f.exec(t, `UPDATE media_artifact_journal SET state='applied' WHERE asset_id=$1 AND operation='write'`, f.asset)
			if err := f.e.ApplyArchiveCleanup(f.ctx, id); err != nil {
				t.Fatal("manual retry after repair failed", err)
			}
			f.operation(t, id, "removed")
			f.preserved(t, f.next)
		})
	}
}
