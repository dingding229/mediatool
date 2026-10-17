package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"sort"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/metadata"
)

type MigrationOperation struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	// Only digests, never STRM contents/file Key, leave this server.
	Digest   string `json:"-"`
	Expected string `json:"-"`
	Scope    string `json:"-"`
}
type MigrationPreview struct {
	AssetID                string               `json:"assetId"`
	Revision               int64                `json:"revision"`
	PlanDigest             string               `json:"planDigest"`
	Operations             []MigrationOperation `json:"operations"`
	CloudFileUnchanged     bool                 `json:"cloudFileUnchanged"`
	PlaybackGrantUnchanged bool                 `json:"playbackGrantUnchanged"`
	OutputProjectionMode   string               `json:"outputProjectionMode"`
	WholeFilePlayback      bool                 `json:"wholeFilePlayback"`
	DeclaredEpisodes       []int                `json:"declaredEpisodes,omitempty"`
	SharedNFORetained      bool                 `json:"sharedNfoRetained"`
}

func migrationPreview(ctx context.Context, tx pgx.Tx, assetID, root string, in Confirmation, intents []string) (MigrationPreview, error) {
	rows, err := tx.Query(ctx, `SELECT relative_path,artifact_kind,operation,content_digest,expected_digest,owner_scope FROM media_artifact_journal WHERE id=ANY($1::uuid[])`, intents)
	if err != nil {
		return MigrationPreview{}, err
	}
	defer rows.Close()
	p := MigrationPreview{WholeFilePlayback: true, OutputProjectionMode: "single_file", DeclaredEpisodes: append([]int(nil), in.Episodes...), AssetID: assetID, Revision: in.Revision, CloudFileUnchanged: true, PlaybackGrantUnchanged: true, SharedNFORetained: true}
	if in.MediaType == "series" {
		for i := 1; i < len(in.Episodes); i++ {
			if in.Episodes[i] != in.Episodes[i-1]+1 {
				p.OutputProjectionMode = "explicit_episode_aliases"
				break
			}
		}
	}
	for rows.Next() {
		var op MigrationOperation
		var digest, expected []byte
		if err = rows.Scan(&op.Path, &op.Kind, &op.Operation, &digest, &expected, &op.Scope); err != nil {
			return p, err
		}
		op.Digest = hex.EncodeToString(digest)
		op.Expected = hex.EncodeToString(expected)
		p.Operations = append(p.Operations, op)
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	sort.Slice(p.Operations, func(i, j int) bool {
		if p.Operations[i].Operation != p.Operations[j].Operation {
			return p.Operations[i].Operation < p.Operations[j].Operation
		}
		return p.Operations[i].Path < p.Operations[j].Path
	})
	type proof struct{ Path, Kind, Operation, Digest, Expected, Scope string }
	proofs := []proof{}
	for _, o := range p.Operations {
		proofs = append(proofs, proof{o.Path, o.Kind, o.Operation, o.Digest, o.Expected, o.Scope})
	}
	input := in
	input.PlanDigest = ""
	raw, err := json.Marshal(struct {
		Asset, Root string
		Input       Confirmation
		Operations  []proof
	}{assetID, root, input, proofs})
	if err != nil {
		return p, err
	}
	sum := sha256.Sum256(raw)
	p.PlanDigest = hex.EncodeToString(sum[:])
	return p, nil
}

// PreviewIdentityMigration stages the exact normal transaction and rolls it
// back. No filesystem write/archive or metadata/grant change is made. Apply
// restages and requires the same proof, so an edit/provider/config change forces
// a fresh preview instead of accepting a stale click.
func (e Engine) PreviewIdentityMigration(ctx context.Context, assetID string, in Confirmation) (MigrationPreview, error) {
	in.MigrateOutputs = true
	if in.MediaType != "movie" && in.MediaType != "series" || in.TMDBID < 1 {
		return MigrationPreview{}, errors.New("verified media identity required")
	}
	if in.MediaType == "series" && (in.Season < 0 || in.Season > 999 || len(in.Episodes) == 0 || len(in.Episodes) > 100) {
		return MigrationPreview{}, errors.New("explicit season and episode coverage required")
	}
	seen := map[int]bool{}
	for _, v := range in.Episodes {
		if v < 1 || v > 9999 || seen[v] {
			return MigrationPreview{}, errors.New("invalid or duplicate episode")
		}
		seen[v] = true
	}
	if in.MediaType == "series" {
		var err error
		in.Episodes, err = metadata.NormalizeEpisodes(in.Season, in.Episodes)
		if err != nil {
			return MigrationPreview{}, err
		}
	}
	provider, err := e.Connections.TMDB(ctx)
	if err != nil {
		return MigrationPreview{}, err
	}
	item, err := provider.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return MigrationPreview{}, err
	}
	if item.ID != in.TMDBID || item.MediaType != in.MediaType {
		return MigrationPreview{}, errors.New("metadata identity mismatch")
	}
	item, err = provider.WithEpisodes(ctx, item, in.Season, in.Episodes)
	if err != nil {
		return MigrationPreview{}, err
	}
	var scrapeSeason *int
	if in.MediaType == "series" {
		scrapeSeason = &in.Season
	}
	item, err = e.enrichScraping(ctx, item, scrapeSeason)
	if err != nil {
		return MigrationPreview{}, err
	}

	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return MigrationPreview{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, assetID); err != nil {
		return MigrationPreview{}, err
	}
	var revision int64
	var cloudState, artifactState, cloudPath string
	if err = tx.QueryRow(ctx, `SELECT a.revision,f.state,a.artifacts_state,f.full_path FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 FOR UPDATE OF a`, assetID).Scan(&revision, &cloudState, &artifactState, &cloudPath); err != nil {
		return MigrationPreview{}, err
	}
	if revision != in.Revision || cloudState != "present" || artifactState != "ready" {
		return MigrationPreview{}, errors.New("asset changed or output recovery still pending")
	}
	outputs, err := correctionOutputs(ctx, tx, assetID)
	if err != nil {
		return MigrationPreview{}, err
	}
	var season, episode *int
	if item.MediaType == "series" {
		season = &in.Season
		episode = &in.Episodes[0]
	}
	intents, err := e.stageOutputMigration(ctx, tx, assetID, cloudPath, item, in, outputs, season, episode)
	if err != nil {
		return MigrationPreview{}, err
	}
	return migrationPreview(ctx, tx, assetID, e.Connections.Config.STRMOutputDir, in, intents)
}

func correctionOutputs(ctx context.Context, tx pgx.Tx, assetID string) ([]oldOutput, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,output_path,key_reference FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outputs := []oldOutput{}
	for rows.Next() {
		var p oldOutput
		if err = rows.Scan(&p.ID, &p.Path, &p.KeyRef); err != nil {
			return nil, err
		}
		outputs = append(outputs, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(outputs) == 0 {
		return nil, errors.New("STRM required before identity correction")
	}
	return outputs, nil
}
