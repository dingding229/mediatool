package library

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func validCleanupPaths(archive, quarantine string) bool {
	return validRelativeArtifact(archive) && validRelativeArtifact(quarantine) && strings.HasPrefix(archive, ".media-tool-retired/") && strings.HasSuffix(archive, ".retired") && path.Dir(archive) == path.Dir(quarantine) && strings.HasPrefix(path.Base(quarantine), ".cleanup-") && strings.HasSuffix(quarantine, ".retired")
}

// quarantineArchive preserves manual/racing changes with no-replace rename.
// Missing files before an operation was durably quarantined are unknown, not
// a claim that the program deleted them. User-created archive paths remain.
func quarantineArchive(root, archive, quarantine string, digest []byte) (bool, error) {
	if !validCleanupPaths(archive, quarantine) {
		return false, ErrArtifactConflict
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	q, qErr := readRegular(fs, quarantine)
	if qErr == nil {
		if !digestEqual(q, digest) {
			return false, ErrArtifactConflict
		}
		if _, err = fs.Lstat(archive); errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, ErrArtifactConflict
	}
	if !errors.Is(qErr, os.ErrNotExist) {
		return false, qErr
	}
	raw, err := readRegular(fs, archive)
	if err != nil || !digestEqual(raw, digest) {
		return false, ErrArtifactConflict
	}
	directory, err := fs.Open(path.Dir(archive))
	if err != nil {
		return false, err
	}
	defer deferutil.Close(directory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err = renameNoReplace(directory, path.Base(archive), directory, path.Base(quarantine)); err != nil {
		return false, err
	}
	moved, err := readRegular(fs, quarantine)
	if err != nil || !digestEqual(moved, digest) {
		_ = renameNoReplace(directory, path.Base(quarantine), directory, path.Base(archive))
		return false, ErrArtifactConflict
	}
	if err = directory.Sync(); err != nil {
		return false, err
	}
	return true, nil
}

// removeQuarantined removes only the explicitly approved, private quarantine
// path. It never touches original outputs, arbitrary expired files or folders.
func removeQuarantined(root, archive, quarantine string, digest []byte) (bool, error) {
	if !validCleanupPaths(archive, quarantine) {
		return false, ErrArtifactConflict
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if _, err = fs.Lstat(archive); !errors.Is(err, os.ErrNotExist) {
		return false, ErrArtifactConflict
	}
	raw, err := readRegular(fs, quarantine)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil || !digestEqual(raw, digest) {
		return false, ErrArtifactConflict
	}
	if err = fs.Remove(quarantine); err != nil {
		return false, err
	}
	directory, err := fs.Open(path.Dir(quarantine))
	if err != nil {
		return false, err
	}
	defer deferutil.Close(directory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	return true, directory.Sync()
}

func (e Engine) ApplyArchiveCleanup(ctx context.Context, id string) error {
	if !ValidArchiveID(id) {
		return errors.New("invalid archive cleanup operation ID")
	}
	id = strings.ToLower(id)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var relative string
	if err = tx.QueryRow(ctx, `SELECT j.relative_path FROM media_archive_cleanup_operations c JOIN media_artifact_journal j ON j.id=c.journal_id WHERE c.id=$1`, id).Scan(&relative); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, relative); err != nil {
		return err
	}
	var state, archive, quarantine, root, journal, asset, scope, kind, journalState, assetState string
	var expected []byte
	var archived time.Time
	var removed *time.Time
	var retention int
	err = tx.QueryRow(ctx, `SELECT c.state,j.archive_relative,c.quarantine_relative,j.root_path,j.id::text,j.asset_id::text,j.owner_scope,j.artifact_kind,j.state,c.expected_digest,j.updated_at,j.archive_deleted_at,a.artifacts_state,p.retention_days FROM media_archive_cleanup_operations c JOIN media_artifact_journal j ON j.id=c.journal_id JOIN media_assets a ON a.id=j.asset_id CROSS JOIN media_archive_policy p WHERE c.id=$1 AND j.operation='archive' FOR UPDATE OF c,j FOR SHARE OF a,p`, id).Scan(&state, &archive, &quarantine, &root, &journal, &asset, &scope, &kind, &journalState, &expected, &archived, &removed, &assetState, &retention)
	if err != nil {
		return err
	}
	if state == "removed" {
		return nil
	}
	configured, err := filepath.Abs(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return err
	}
	validPath := path.Join(".media-tool-retired", journal, relative+".retired")
	if root != configured || archive != validPath || scope != "asset:"+asset || journalState != "applied" || removed != nil || path.Base(relative) == "tvshow.nfo" || assetState != "ready" || time.Now().Before(archived.Add(time.Duration(retention)*24*time.Hour)) {
		return e.cleanupConflict(ctx, tx, id, "归档归属、保留期或资产状态已变化；未执行清理")
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artifact_journal WHERE asset_id=$1 AND state<>'applied')`, asset).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return e.cleanupConflict(ctx, tx, id, "资产仍有待恢复输出；未执行清理")
	}
	if err = activeArchiveOutputs(ctx, tx, asset, root); err != nil {
		return e.cleanupConflict(ctx, tx, id, "当前托管STRM缺失或已修改；保留归档，请先恢复输出")
	}
	if state != "quarantined" {
		if err = dbutil.CheckEffect(ctx); err != nil {
			return err
		}
		ok, moveErr := quarantineArchive(root, archive, quarantine, expected)
		if moveErr != nil || !ok {
			return e.cleanupConflict(ctx, tx, id, "归档文件已修改/缺失或隔离目标冲突；保留文件，请核对后明确重试")
		}
		if _, err = tx.Exec(ctx, `UPDATE media_archive_cleanup_operations SET state='quarantined',message='',updated_at=now()WHERE id=$1`, id); err != nil {
			return err
		}
		if err = dbutil.Commit(ctx, tx); err != nil {
			return err
		}
		// Persisted phase is required before unlink. Cancellation stops here and is
		// later reconciled only by this explicit approval's retry endpoint.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return e.ApplyArchiveCleanup(ctx, id)
	}
	if err = dbutil.CheckEffect(ctx); err != nil {
		return err
	}
	done, removeErr := removeQuarantined(root, archive, quarantine, expected)
	if removeErr != nil || !done {
		return e.cleanupConflict(ctx, tx, id, "隔离文件或归档路径已变化；未继续删除，请人工核对")
	}
	if _, err = tx.Exec(ctx, `UPDATE media_archive_cleanup_operations SET state='removed',message='',updated_at=now()WHERE id=$1`, id); err != nil {
		return err
	}
	// Keep the immutable archive's original completion time for audit/retention.
	if _, err = tx.Exec(ctx, `UPDATE media_artifact_journal SET archive_deleted_at=now()WHERE id=$1`, journal); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','archive.cleanup_completed',jsonb_build_object('cleanupId',$1::text,'archiveId',$2::text,'kind',$3::text))`, id, journal, kind); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
func (e Engine) cleanupConflict(ctx context.Context, tx pgx.Tx, id, message string) error {
	if _, err := tx.Exec(ctx, `UPDATE media_archive_cleanup_operations SET state='conflict',message=$2,updated_at=now()WHERE id=$1`, id, message); err != nil {
		return err
	}
	if err := dbutil.Commit(ctx, tx); err != nil {
		return err
	}
	return ErrArtifactConflict
}
