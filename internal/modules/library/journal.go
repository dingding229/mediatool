package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/artifacts"
	"media-tool/internal/platform/secrets"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrArtifactConflict = errors.New("artifact changed or belongs to another owner; resolve manually without overwriting")

type Journal struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

// Stage belongs to the caller's business transaction. No file is written before
// its encrypted intent and grant/asset records have committed.
func (j Journal) Stage(ctx context.Context, tx pgx.Tx, root, relative, assetID, scope, kind string, content []byte) (string, error) {
	if root == "" {
		return "", errors.New("artifact root is required")
	}
	absolute, rootErr := filepath.Abs(root)
	if rootErr != nil {
		return "", errors.New("unsafe artifact root")
	}
	root = absolute
	if !path.IsAbs(root) || relative == "" || path.IsAbs(relative) || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") || strings.ContainsAny(relative, "\\\x00") || len(content) > 20<<20 {
		return "", errors.New("unsafe artifact intent")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, relative); err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	var pending, pendingScope, pendingRoot string
	var pendingDigest []byte
	err := tx.QueryRow(ctx, `SELECT id::text,owner_scope,root_path,content_digest FROM media_artifact_journal WHERE relative_path=$1 AND state IN ('pending','conflict') FOR UPDATE`, relative).Scan(&pending, &pendingScope, &pendingRoot, &pendingDigest)
	if err == nil {
		if scope != pendingScope || root != pendingRoot || !bytes.Equal(pendingDigest, digest[:]) {
			return "", ErrArtifactConflict
		}
		return pending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var old []byte
	var oldScope string
	err = tx.QueryRow(ctx, `SELECT owner_scope,content_digest FROM managed_media_artifacts WHERE relative_path=$1 FOR UPDATE`, relative).Scan(&oldScope, &old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if oldScope != "" && oldScope != scope {
		// Shared tvshow metadata may be adopted from pre-journal versions only when
		// content is unchanged; unrelated assets never acquire ownership.
		if kind != "nfo" || path.Base(relative) != "tvshow.nfo" || !bytes.Equal(old, digest[:]) {
			return "", ErrArtifactConflict
		}
	}

	id := jobs.UUID()
	encrypted, err := j.Vault.Seal(id, map[string]string{"content": base64.StdEncoding.EncodeToString(content)})
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_artifact_journal(id,relative_path,root_path,asset_id,owner_scope,artifact_kind,content_ciphertext,content_digest,expected_digest)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, relative, root, assetID, scope, kind, encrypted, digest[:], old)
	return id, err
}
func (j Journal) Apply(ctx context.Context, id string) error {
	tx, err := j.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var relative string
	if err = tx.QueryRow(ctx, `SELECT relative_path FROM media_artifact_journal WHERE id=$1`, id).Scan(&relative); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, relative); err != nil {
		return err
	}
	var root, asset, scope, kind, state, operation string
	var archive *string
	var requires []string
	var encrypted, digest, previous []byte
	if err = tx.QueryRow(ctx, `SELECT root_path,asset_id::text,owner_scope,artifact_kind,state,content_ciphertext,content_digest,expected_digest,operation,archive_relative,requires_intents FROM media_artifact_journal WHERE id=$1 FOR UPDATE`, id).Scan(&root, &asset, &scope, &kind, &state, &encrypted, &digest, &previous, &operation, &archive, &requires); err != nil {
		return err
	}
	if state == "applied" {
		return nil
	}
	if operation == "archive" {
		if archive == nil {
			return ErrArtifactConflict
		}
		return j.applyArchive(ctx, tx, id, root, relative, asset, scope, kind, *archive, previous, requires)
	}
	values, err := j.Vault.Open(id, encrypted)
	if err != nil {
		return err
	}
	content, err := base64.StdEncoding.DecodeString(values["content"])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(content)
	if !bytes.Equal(sum[:], digest) {
		return errors.New("corrupt artifact intent")
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	f, openErr := fs.Open(relative)
	if openErr == nil {
		raw, readErr := io.ReadAll(io.LimitReader(f, (20<<20)+1))
		f.Close()
		if readErr != nil {
			fs.Close()
			return readErr
		}
		current := sha256.Sum256(raw)
		if !bytes.Equal(current[:], digest) && !bytes.Equal(current[:], previous) {
			fs.Close()
			tx.Exec(ctx, `UPDATE media_artifact_journal SET state='conflict',updated_at=now()WHERE id=$1`, id)
			if err = tx.Commit(ctx); err != nil {
				return err
			}
			return ErrArtifactConflict
		}
	} else if !errors.Is(openErr, os.ErrNotExist) {
		fs.Close()
		return openErr
	}
	fs.Close()
	if err = artifacts.Write(root, relative, content); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,content_digest,owner_scope)VALUES($1,$2,$3,$4,$5)ON CONFLICT(relative_path)DO UPDATE SET content_digest=excluded.content_digest,owner_scope=excluded.owner_scope,updated_at=now()`, relative, asset, kind, digest, scope); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_artifact_journal SET state='applied',content_ciphertext=NULL,updated_at=now()WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (j Journal) Write(ctx context.Context, root, relative, assetID, scope, kind string, content []byte) error {
	tx, err := j.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	id, err := j.Stage(ctx, tx, root, relative, assetID, scope, kind, content)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return j.Apply(ctx, id)
}
func (e Engine) RecoverArtifacts(ctx context.Context) error {
	j := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM media_artifact_journal WHERE state='pending' ORDER BY CASE WHEN operation='write' THEN 0 ELSE 1 END,created_at,id LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = j.Apply(ctx, id)
	}
	return e.FinalizeRecovered(ctx)
}

// Finalize only candidates whose complete staged manifest has been applied.
// Download imports retain their Worker retry path so subtitles are not skipped.
func (e Engine) FinalizeRecovered(ctx context.Context) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT c.id::text,c.asset_id::text FROM library_candidates c WHERE state IN ('artifacts_pending','needs_output_reconciliation') AND EXISTS(SELECT 1 FROM media_artifact_journal j WHERE j.asset_id=c.asset_id AND j.state='applied' AND j.artifact_kind='nfo') AND NOT EXISTS(SELECT 1 FROM media_artifact_journal j WHERE j.asset_id=c.asset_id AND j.state<>'applied') FOR UPDATE OF c`)
	if err != nil {
		return err
	}
	type candidate struct{ id, asset string }
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.asset); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		var kind string
		var season *int
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT m.media_type,b.season,b.episodes FROM media_asset_bindings b JOIN media_items m ON m.id=b.media_id WHERE b.asset_id=$1`, c.asset).Scan(&kind, &season, &raw); err != nil {
			return err
		}
		var episodes []int
		if json.Unmarshal(raw, &episodes) != nil {
			return ErrOutputLayoutRequired
		}
		paths := []string{}
		out, err := tx.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, c.asset)
		if err != nil {
			return err
		}
		for out.Next() {
			var p string
			if err = out.Scan(&p); err != nil {
				out.Close()
				return err
			}
			paths = append(paths, p)
		}
		err = out.Err()
		out.Close()
		if err != nil {
			return err
		}
		if _, layoutErr := ExistingOutputProjections(paths, kind, season, episodes); layoutErr != nil {
			if _, err = tx.Exec(ctx, `UPDATE library_candidates SET state='needs_output_reconciliation',revision=revision+CASE WHEN state='needs_output_reconciliation' THEN 0 ELSE 1 END,updated_at=now()WHERE id=$1`, c.id); err != nil {
				return err
			}
			continue
		}
		if workingErr := activeArchiveOutputs(ctx, tx, c.asset, e.Connections.Config.STRMOutputDir); workingErr != nil {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE library_candidates SET state='confirmed',updated_at=now()WHERE id=$1`, c.id); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"assetId": c.asset})
		if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "library.recovered:"+c.asset); err != nil {
			return err
		}
	}
	recovered, err := tx.Query(ctx, `UPDATE media_assets a SET artifacts_state='ready'WHERE artifacts_state='pending' AND NOT EXISTS(SELECT 1 FROM media_artifact_journal j WHERE j.asset_id=a.id AND j.state<>'applied') RETURNING id::text,revision`)
	if err != nil {
		return err
	}
	type assetRevision struct {
		id       string
		revision int64
	}
	changed := []assetRevision{}
	for recovered.Next() {
		var asset assetRevision
		if err = recovered.Scan(&asset.id, &asset.revision); err != nil {
			recovered.Close()
			return err
		}
		changed = append(changed, asset)
	}
	err = recovered.Err()
	recovered.Close()
	if err != nil {
		return err
	}
	for _, asset := range changed {
		payload, _ := json.Marshal(map[string]string{"assetId": asset.id})
		if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, fmt.Sprintf("library.recovered:%s:%d", asset.id, asset.revision)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
