package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
)

var ErrArtifactDependency = errors.New("new output artifacts are not ready; old outputs are retained")

func validRelativeArtifact(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != "." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00")
}

// StageArchive stages only this asset's managed outputs. Shared series NFO is
// deliberately not eligible. All new write intents must finish before retiring
// a source; archive files use .retired suffixes, not playable STRM/media names.
func (j Journal) StageArchive(ctx context.Context, tx pgx.Tx, root, relative, assetID, kind string, requires []string) (string, error) {
	if root == "" || !validRelativeArtifact(relative) || len(requires) == 0 || path.Base(relative) == "tvshow.nfo" {
		return "", ErrArtifactConflict
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, relative); err != nil {
		return "", err
	}
	var scope, owner, ownedKind string
	var digest []byte
	if err = tx.QueryRow(ctx, `SELECT owner_scope,asset_id::text,artifact_kind,content_digest FROM managed_media_artifacts WHERE relative_path=$1 FOR UPDATE`, relative).Scan(&scope, &owner, &ownedKind, &digest); err != nil {
		return "", ErrArtifactConflict
	}
	if scope != "asset:"+assetID || owner != assetID || ownedKind != kind || len(digest) != sha256.Size {
		return "", ErrArtifactConflict
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artifact_journal WHERE relative_path=$1 AND state IN ('pending','conflict'))`, relative).Scan(&pending); err != nil {
		return "", err
	}
	if pending {
		return "", ErrArtifactConflict
	}
	id := jobs.UUID()
	archive := path.Join(".media-tool-retired", id, relative+".retired")
	_, err = tx.Exec(ctx, `INSERT INTO media_artifact_journal(id,relative_path,root_path,asset_id,owner_scope,artifact_kind,content_digest,expected_digest,operation,archive_relative,requires_intents) VALUES($1,$2,$3,$4,$5,$6,$7,$7,'archive',$8,$9::uuid[])`, id, relative, absolute, assetID, scope, kind, digest, archive, requires)
	return id, err
}

// readRegular refuses final symlinks/non-files and bounds every artifact read.
func readRegular(fs *os.Root, relative string) ([]byte, error) {
	info, err := fs.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 20<<20 {
		return nil, ErrArtifactConflict
	}
	file, err := fs.Open(relative)
	if err != nil {
		return nil, err
	}
	defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return nil, ErrArtifactConflict
	}
	raw, err := io.ReadAll(io.LimitReader(file, (20<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 20<<20 {
		return nil, ErrArtifactConflict
	}
	return raw, nil
}
func digestEqual(raw, digest []byte) bool {
	sum := sha256.Sum256(raw)
	return bytes.Equal(sum[:], digest)
}

// archiveFile is recoverable after rename-before-commit. Original output is
// never unlinked without preserving its actual bytes in an archive. A race/manual
// edit found after rename is retained and restored only without replacing a new
// user file. Conflicting archives or recreated originals are never overwritten.
func archiveFile(root, relative, archive string, expected []byte) error {
	if !validRelativeArtifact(relative) || !validRelativeArtifact(archive) || !strings.HasPrefix(archive, ".media-tool-retired/") || !strings.HasSuffix(archive, ".retired") {
		return ErrArtifactConflict
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	saved, savedErr := readRegular(fs, archive)
	if savedErr == nil {
		if !digestEqual(saved, expected) {
			return ErrArtifactConflict
		}
		if _, err = fs.Lstat(relative); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return ErrArtifactConflict
	}
	if !errors.Is(savedErr, os.ErrNotExist) {
		return savedErr
	}
	raw, err := readRegular(fs, relative)
	if err != nil {
		return err
	}
	if !digestEqual(raw, expected) {
		return ErrArtifactConflict
	}
	if err = fs.MkdirAll(path.Dir(archive), 0750); err != nil {
		return err
	}
	// Each UUID directory belongs to this journal intent; reject any preexisting
	// destination instead of adopting or overwriting a different archive.
	if _, err = fs.Lstat(archive); !errors.Is(err, os.ErrNotExist) {
		return ErrArtifactConflict
	}
	sourceDirectory, err := fs.Open(path.Dir(relative))
	if err != nil {
		return err
	}
	defer deferutil.Close(sourceDirectory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	archiveDirectory, err := fs.Open(path.Dir(archive))
	if err != nil {
		return err
	}
	defer deferutil.Close(archiveDirectory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err = renameNoReplace(sourceDirectory, path.Base(relative), archiveDirectory, path.Base(archive)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrArtifactConflict
		}
		return err
	}
	moved, err := readRegular(fs, archive)
	if err != nil || !digestEqual(moved, expected) {
		// Link never replaces an original created after the move. Keep the archive
		// regardless, so no manual content is discarded on an unknown race outcome.
		_ = fs.Link(archive, relative)
		return ErrArtifactConflict
	}
	for _, directory := range []string{path.Dir(relative), path.Dir(archive)} {
		dir, err := fs.Open(directory)
		if err != nil {
			return err
		}
		syncErr := dir.Sync()
		deferutil.Close(dir, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if syncErr != nil {
			return syncErr
		}
	}
	return nil
}

func (j Journal) applyArchive(ctx context.Context, tx pgx.Tx, id, root, relative, asset, scope, kind, archive string, expected []byte, requires []string) error {
	rows, err := tx.Query(ctx, `SELECT relative_path,root_path,asset_id::text,owner_scope,artifact_kind,state,operation,content_digest FROM media_artifact_journal WHERE id=ANY($1::uuid[])`, requires)
	if err != nil {
		return err
	}
	type dependency struct {
		relative, root, asset, scope, kind, state, operation string
		digest                                               []byte
	}
	dependencies := []dependency{}
	for rows.Next() {
		var d dependency
		if err = rows.Scan(&d.relative, &d.root, &d.asset, &d.scope, &d.kind, &d.state, &d.operation, &d.digest); err != nil {
			rows.Close()
			return err
		}
		dependencies = append(dependencies, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(requires) == 0 || len(dependencies) != len(requires) {
		return ErrArtifactDependency
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	for _, d := range dependencies {
		shared := d.kind == "nfo" && path.Base(d.relative) == "tvshow.nfo" && strings.HasPrefix(d.scope, "tmdb:series:")
		if d.state != "applied" || d.operation != "write" || d.root != root || (d.asset != asset && !shared) {
			return ErrArtifactDependency
		}
		raw, readErr := readRegular(fs, d.relative)
		if readErr != nil || !digestEqual(raw, d.digest) {
			return ErrArtifactDependency
		}
	}

	var currentScope, currentOwner, currentKind string
	var currentDigest []byte
	if err := tx.QueryRow(ctx, `SELECT owner_scope,asset_id::text,artifact_kind,content_digest FROM managed_media_artifacts WHERE relative_path=$1 FOR UPDATE`, relative).Scan(&currentScope, &currentOwner, &currentKind, &currentDigest); err != nil {
		return ErrArtifactConflict
	}
	if scope != "asset:"+asset || currentScope != scope || currentOwner != asset || currentKind != kind || !bytes.Equal(currentDigest, expected) {
		return ErrArtifactConflict
	}
	err = archiveFile(root, relative, archive, expected)
	if errors.Is(err, ErrArtifactConflict) {
		if _, saveErr := tx.Exec(ctx, `UPDATE media_artifact_journal SET state='conflict',updated_at=now() WHERE id=$1`, id); saveErr != nil {
			return saveErr
		}
		if saveErr := tx.Commit(ctx); saveErr != nil {
			return saveErr
		}
		return err
	}
	if err != nil {
		return err
	}
	if kind == "strm" {
		if _, err = tx.Exec(ctx, `DELETE FROM strm_entries WHERE output_path=$1 AND asset_id=$2`, relative, asset); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM managed_media_artifacts WHERE relative_path=$1 AND asset_id=$2 AND owner_scope=$3 AND content_digest=$4`, relative, asset, scope, expected); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_artifact_journal SET state='applied',content_ciphertext=NULL,updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
