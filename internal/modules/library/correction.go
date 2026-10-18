package library

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
)

// Correct retains assetId/grant and cloud/source media. Output path changes
// require an explicit migrateOutputs opt-in; old managed files are archived.
// Every NFO intent is staged in the binding transaction. Shared tvshow conflicts
// reject the entire correction rather than changing neighbouring episodes.
func (e Engine) Correct(ctx context.Context, assetID string, in Confirmation) error {
	if in.MediaType != "movie" && in.MediaType != "series" || in.TMDBID < 1 {
		return errors.New("verified media identity required")
	}
	if in.MediaType == "series" && (in.Season < 0 || in.Season > 999 || len(in.Episodes) < 1 || len(in.Episodes) > 100) {
		return errors.New("season and explicit episode coverage required")
	}
	seen := map[int]bool{}
	for _, ep := range in.Episodes {
		if ep < 1 || ep > 9999 || seen[ep] {
			return errors.New("invalid or duplicate episode")
		}
		seen[ep] = true
	}
	if in.MediaType == "series" {
		var err error
		in.Episodes, err = metadata.NormalizeEpisodes(in.Season, in.Episodes)
		if err != nil {
			return err
		}
	}
	provider, err := e.Connections.TMDB(ctx)
	if err != nil {
		return err
	}
	item, err := provider.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return err
	}
	if item.ID != in.TMDBID || item.MediaType != in.MediaType {
		return errors.New("metadata provider returned a different identity")
	}
	item, err = provider.WithEpisodes(ctx, item, in.Season, in.Episodes)
	if err != nil {
		return err
	}
	var scrapeSeason *int
	if in.MediaType == "series" {
		scrapeSeason = &in.Season
	}
	item, err = e.enrichScraping(ctx, item, scrapeSeason)
	if err != nil {
		return err
	}

	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if err = checkScrapeSettings(ctx, tx, item); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, assetID); err != nil {
		return err
	}
	var revision int64
	var state, artifactsState, cloudPath string
	if err = tx.QueryRow(ctx, `SELECT a.revision,f.state,a.artifacts_state,f.full_path FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 FOR UPDATE OF a`, assetID).Scan(&revision, &state, &artifactsState, &cloudPath); err != nil {
		return err
	}
	if revision != in.Revision || state != "present" || artifactsState != "ready" {
		return errors.New("asset changed or missing; reload before correction")
	}
	outputs, err := correctionOutputs(ctx, tx, assetID)
	if err != nil {
		return err
	}

	var season, episode *int
	episodes := []int{}
	if in.MediaType == "series" {
		season = &in.Season
		episode = &in.Episodes[0]
		episodes = in.Episodes
	}
	journal := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	intents := []string{}
	if in.MigrateOutputs {
		var err error
		intents, err = e.stageOutputMigration(ctx, tx, assetID, cloudPath, item, in, outputs, season, episode)
		if err != nil {
			return err
		}
		preview, err := migrationPreview(ctx, tx, assetID, e.Connections.Config.STRMOutputDir, in, intents)
		if err != nil {
			return err
		}
		if in.PlanDigest == "" || preview.PlanDigest != in.PlanDigest {
			return errors.New("output migration preview changed or missing; preview again before confirmation")
		}
	} else {
		mapped := map[string]OutputProjection{}
		if item.MediaType == "series" {
			var err error
			mapped, err = correctionProjections(outputs, in.Season, in.Episodes)
			if err != nil {
				return err
			}
		}
		for _, output := range outputs {
			var existingType string
			if err = tx.QueryRow(ctx, `SELECT m.media_type FROM media_asset_bindings b JOIN media_items m ON m.id=b.media_id WHERE b.asset_id=$1`, assetID).Scan(&existingType); err != nil || existingType != item.MediaType {
				return errors.New("媒体类型改变，需要明确迁移输出；目录名称不是身份凭据")
			}

			projection := OutputProjection{Path: output.Path, WholeFilePlayback: true}
			if item.MediaType == "series" {
				projection = mapped[output.Path]
			}
			ids, err := StageNFOProjection(ctx, journal, tx, e.Connections.Config.STRMOutputDir, assetID, item, season, projection)
			if err != nil {
				return err
			}
			intents = append(intents, ids...)
			if extra, err := StageScrapeExtras(ctx, journal, tx, e.Connections.Config.STRMOutputDir, assetID, item, season, projection); err != nil {
				return err
			} else {
				intents = append(intents, extra...)
			}
		}
	}

	raw, _ := json.Marshal(item)
	showRaw, _ := json.Marshal(item.ShowMetadata())
	var mediaID string
	if err = tx.QueryRow(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(media_type,tmdb_id)DO UPDATE SET metadata=excluded.metadata,title=excluded.title,release_year=excluded.release_year RETURNING id::text`, jobs.UUID(), item.MediaType, item.ID, item.Title, item.Year, showRaw).Scan(&mediaID); err != nil {
		return err
	}
	eps, _ := json.Marshal(episodes)
	if _, err = tx.Exec(ctx, `INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)VALUES($1,$2,$3,$4,$5)ON CONFLICT(asset_id)DO UPDATE SET media_id=excluded.media_id,season=excluded.season,episode=excluded.episode,episodes=excluded.episodes`, assetID, mediaID, season, episode, eps); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_assets SET media_identity=$2,revision=revision+1,artifacts_state='pending'WHERE id=$1`, assetID, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','library.identity_corrected',jsonb_build_object('assetId',$1::text,'tmdbId',$2::int,'migrateOutputs',$3::bool))`, assetID, in.TMDBID, in.MigrateOutputs); err != nil {
		return err
	}
	if err = saveScrapeReport(ctx, tx, assetID, item); err != nil {
		return err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return err
	}
	for _, id := range intents {
		if err = journal.Apply(ctx, id); err != nil {
			return err
		}
	}
	if _, err = dbutil.Exec(ctx, e.DB, `UPDATE media_assets SET artifacts_state='ready'WHERE id=$1 AND revision=$2`, assetID, revision+1); err != nil {
		return err
	}
	return e.QueueRefresh(ctx, assetID)
}
