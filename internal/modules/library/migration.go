package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
)

type oldOutput struct{ ID, Path, KeyRef string }
type ownedOutput struct {
	Path, Kind string
	Digest     []byte
	Content    []byte
}

func (e Engine) stageOutputMigration(ctx context.Context, tx pgx.Tx, assetID, cloudPath string, item metadata.Item, in Confirmation, outputs []oldOutput, season, episode *int) ([]string, error) {
	root := e.Connections.Config.STRMOutputDir
	rows, err := tx.Query(ctx, `SELECT relative_path,artifact_kind,content_digest FROM managed_media_artifacts WHERE asset_id=$1 AND owner_scope=$2 ORDER BY relative_path FOR UPDATE`, assetID, "asset:"+assetID)
	if err != nil {
		return nil, err
	}
	managed := map[string]ownedOutput{}
	for rows.Next() {
		var a ownedOutput
		if err = rows.Scan(&a.Path, &a.Kind, &a.Digest); err != nil {
			rows.Close()
			return nil, err
		}
		managed[a.Path] = a
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	j := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	writes := []string{}
	retire := map[string]ownedOutput{}
	targets := map[string]bool{}
	projections, err := AssetOutputProjections(cloudPath, assetID, item, season, in.Episodes)
	if err != nil {
		return nil, err
	}
	projections, err = applyTemplatePaths(ctx, e.DB, cloudPath, assetID, item, projections)
	if err != nil {
		return nil, err
	}
	if len(outputs) == 0 || len(outputs) > 100 {
		return nil, ErrArtifactConflict
	}
	// Alias entries represent one cloud asset, not different editions. Merge only
	// when every verified original has exactly the same fixed URL/key reference.
	var sourceContent []byte
	keyRef := ""
	subtitles := map[string][]byte{}
	for _, old := range outputs {
		a, ok := managed[old.Path]
		if !ok || a.Kind != "strm" {
			return nil, errors.New("old STRM has no trusted ownership manifest; reconcile before migrating")
		}
		content, err := readRegular(fs, old.Path)
		if err != nil || !digestEqual(content, a.Digest) {
			return nil, ErrArtifactConflict
		}
		if sourceContent == nil {
			sourceContent = content
			keyRef = old.KeyRef
		} else if !bytes.Equal(sourceContent, content) || keyRef != old.KeyRef {
			return nil, errors.New("同一资产的旧STRM内容或授权引用不同，请先核对/明确轮换播放Key，不自动选择其中一个")
		}
		retire[old.Path] = a
		stem := strings.TrimSuffix(old.Path, path.Ext(old.Path))
		if nfo, ok := managed[stem+".nfo"]; ok {
			retire[nfo.Path] = nfo
		}
		for name, sidecar := range managed {
			if sidecar.Kind != "subtitle" || path.Dir(name) != path.Dir(old.Path) || !strings.HasPrefix(name, stem+".") {
				continue
			}
			raw, err := readRegular(fs, name)
			if err != nil || !digestEqual(raw, sidecar.Digest) {
				return nil, ErrArtifactConflict
			}
			suffix := strings.TrimPrefix(name, stem)
			if previous, ok := subtitles[suffix]; ok && !bytes.Equal(previous, raw) {
				return nil, errors.New("同一合集的字幕别名内容不同，请人工核对后再迁移")
			}
			subtitles[suffix] = raw
			retire[name] = sidecar
		}
	}
	var total int64
	for _, raw := range subtitles {
		total += int64(len(raw))
	}
	if len(projections)*(len(subtitles)+2)+1 > 500 || total*int64(len(projections)) > 64<<20 {
		return nil, errors.New("output projection subtitle amplification exceeds 500 files/64MiB")
	}
	for _, projection := range projections {
		target := projection.Path
		if target == "" || targets[target] {
			return nil, ErrArtifactConflict
		}
		targets[target] = true
		var occupied string
		err = tx.QueryRow(ctx, `SELECT asset_id::text FROM strm_entries WHERE output_path=$1 FOR UPDATE`, target).Scan(&occupied)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if occupied != "" && occupied != assetID {
			return nil, ErrArtifactConflict
		}
		id, err := stageMigrationWrite(ctx, j, tx, root, target, assetID, "asset:"+assetID, "strm", sourceContent)
		if err != nil {
			return nil, err
		}
		writes = append(writes, id)
		if _, err = tx.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)ON CONFLICT(output_path)DO UPDATE SET updated_at=now()WHERE strm_entries.asset_id=excluded.asset_id`, jobs.UUID(), assetID, target, keyRef); err != nil {
			return nil, err
		}
		delete(retire, target)
		nfos, err := nfoProjectionFiles(target, assetID, item, season, projection.Episodes, projection.SourceEpisodes)
		if err != nil {
			return nil, err
		}
		for _, nfo := range selectedNFO(nfos, assetID, item) {
			id, err := stageMigrationWrite(ctx, j, tx, root, nfo.relative, assetID, nfo.scope, "nfo", nfo.data)
			if err != nil {
				return nil, err
			}
			writes = append(writes, id)
			delete(retire, nfo.relative)
		}
		extras, err := scrapeFiles(target, assetID, item, season, projection.Episodes)
		if err != nil {
			return nil, err
		}
		for _, extra := range extras {
			id, err := stageMigrationWrite(ctx, j, tx, root, extra.Path, assetID, extra.Scope, extra.Kind, extra.Data)
			if err != nil {
				return nil, err
			}
			writes = append(writes, id)
			delete(retire, extra.Path)
		}
		stem := strings.TrimSuffix(target, path.Ext(target))
		suffixes := []string{}
		for suffix := range subtitles {
			suffixes = append(suffixes, suffix)
		}
		sort.Strings(suffixes)
		for _, suffix := range suffixes {
			next := stem + suffix
			id, err := stageMigrationWrite(ctx, j, tx, root, next, assetID, "asset:"+assetID, "subtitle", subtitles[suffix])
			if err != nil {
				return nil, err
			}
			writes = append(writes, id)
			delete(retire, next)
		}
	}
	// An owned old NFO still receives a hash check before any new identity commits.
	for name, owned := range retire {
		if owned.Kind == "artwork" {
			delete(retire, name)
		}
	}
	names := []string{}
	for name, a := range retire {
		if path.Base(name) == "tvshow.nfo" {
			return nil, ErrArtifactConflict
		}
		raw, err := readRegular(fs, name)
		if err != nil || !digestEqual(raw, a.Digest) {
			return nil, ErrArtifactConflict
		}
		names = append(names, name)
	}
	sort.Strings(names)
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range writes {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	writes = unique
	archives := []string{}
	for _, name := range names {
		a := retire[name]
		id, err := j.StageArchive(ctx, tx, root, name, assetID, a.Kind, writes)
		if err != nil {
			return nil, err
		}
		archives = append(archives, id)
	}
	return append(writes, archives...), nil
}

func stageMigrationWrite(ctx context.Context, j Journal, tx pgx.Tx, root, relative, asset, scope, kind string, content []byte) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, relative); err != nil {
		return "", err
	}
	var previous []byte
	var ownerScope string
	err := tx.QueryRow(ctx, `SELECT owner_scope,content_digest FROM managed_media_artifacts WHERE relative_path=$1 FOR UPDATE`, relative).Scan(&ownerScope, &previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer fs.Close()
	raw, readErr := readRegular(fs, relative)
	if readErr == nil {
		if ownerScope == "" || !digestEqual(raw, previous) {
			return "", ErrArtifactConflict
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	return j.Stage(ctx, tx, root, relative, asset, scope, kind, content)
}
