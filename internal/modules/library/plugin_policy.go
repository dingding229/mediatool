package library

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/metadata"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	dbutil "media-tool/internal/platform/database"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PathsUnderRoot is a read-only prepare/commit check. Atomic artifact writers
// still use os.Root and enforce ownership/digest/layout at staging/application.
func PathsUnderRoot(root string, paths []string) bool {
	for _, relative := range paths {
		if !contracts.RelativeOutput(relative) {
			return false
		}
		p := root
		parts := append([]string{""}, strings.Split(relative, "/")...)
		for _, part := range parts {
			p = filepath.Join(p, part)
			info, e := os.Lstat(p)
			if os.IsNotExist(e) {
				break
			}
			if e != nil || info.Mode()&os.ModeSymlink != 0 {
				return false
			}
		}
	}
	return true
}
func (e Engine) PrepareOutputs(ctx context.Context, id, asset string, projections []OutputProjection) (context.Context, []OutputProjection, error) {
	store := pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}
	q := contracts.TransferPlan{OperationID: id, Paths: []string{}}
	for _, p := range projections {
		q.Paths = append(q.Paths, p.Path)
	}
	ctx, err := store.TransferDecision(ctx, contracts.ChainTransferIntercept, q)
	if err != nil {
		return ctx, nil, err
	}
	var existing bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM strm_entries WHERE asset_id=$1)`, asset).Scan(&existing); err != nil {
		return ctx, nil, err
	}
	original := append([]string{}, q.Paths...)
	validate := func(checkCtx context.Context, paths []string) bool {
		if existing && !slices.Equal(paths, original) {
			return false
		} // Existing outputs require explicit migration.
		if !e.proposalOwners(checkCtx, asset, paths) {
			return false
		}
		if !PathsUnderRoot(e.Connections.Config.STRMOutputDir, paths) {
			return false
		}
		for i, p := range paths {
			s, episodes, ok := metadata.Episodes(filepath.Base(projections[i].Path))
			if ok {
				next, coverage, valid := metadata.Episodes(filepath.Base(p))
				if !valid || next != s || !slices.Equal(episodes, coverage) {
					return false
				}
			}
		}
		return true
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainTransferRenameBuild, contracts.ChainTransferRename} {
		ctx, q.Paths, err = store.TransferPaths(ctx, typ, q, validate)
		if err != nil {
			return ctx, nil, err
		}
	}
	ctx, err = store.TransferDecision(ctx, contracts.ChainTransferOverwriteCheck, q)
	if err != nil {
		return ctx, nil, err
	}
	out := append([]OutputProjection{}, projections...)
	for i := range out {
		out[i].Path = q.Paths[i]
	}
	return ctx, out, nil
}

// Fence renderer inputs captured BEFORE template rendering/RPC. A concurrent
// classification/settings edit cannot commit an old proposal into a new layout.
func (e Engine) PrepareOutputSettings(ctx context.Context) (context.Context, error) {
	var settingsRevision, classificationRevision int64
	if err := e.DB.QueryRow(ctx, `SELECT s.revision,c.revision FROM application_settings s CROSS JOIN directory_classification c WHERE s.singleton AND c.singleton`).Scan(&settingsRevision, &classificationRevision); err != nil {
		return ctx, err
	}
	return dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		var currentSettings, currentClassification int64
		if err := tx.QueryRow(c, `SELECT s.revision,c.revision FROM application_settings s CROSS JOIN directory_classification c WHERE s.singleton AND c.singleton FOR SHARE OF s,c`).Scan(&currentSettings, &currentClassification); err != nil {
			return err
		}
		if currentSettings != settingsRevision || currentClassification != classificationRevision {
			return ErrOutputLayoutRequired
		}
		return nil
	}), nil
}

func (e Engine) proposalOwners(ctx context.Context, asset string, paths []string) bool {
	root, err := filepath.Abs(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return false
	}
	rows, err := e.DB.Query(ctx, `SELECT output_path,asset_id::text,'strm' FROM strm_entries WHERE output_path=ANY($1::text[]) UNION ALL SELECT relative_path,replace(owner_scope,'asset:',''),artifact_kind FROM managed_media_artifacts WHERE relative_path=ANY($1::text[]) UNION ALL SELECT relative_path,CASE WHEN root_path=$2 THEN replace(owner_scope,'asset:','') ELSE '' END,artifact_kind FROM media_artifact_journal WHERE state IN('pending','conflict') AND relative_path=ANY($1::text[])`, paths, root)
	if err != nil {
		return false
	}
	owned := map[string]bool{}
	valid := true
	for rows.Next() {
		var p, owner, kind string
		if err = rows.Scan(&p, &owner, &kind); err != nil {
			valid = false
			break
		}
		if owner != asset || kind != "strm" {
			valid = false
			break
		}
		owned[p] = true
	}
	valid = valid && rows.Err() == nil
	rows.Close()
	if !valid {
		return false
	}
	for _, p := range paths {
		_, err := os.Lstat(filepath.Join(e.Connections.Config.STRMOutputDir, p))
		if err == nil && !owned[p] {
			return false
		}
		if err != nil && !os.IsNotExist(err) {
			return false
		}
	}
	return true
}
