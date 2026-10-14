package library

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/settings"
	"path"
)

// Only NEW output proposals use the configured templates. Existing entries and
// identities remain unchanged until the explicit migration workflow is approved.
func ApplyOutputTemplates(ctx context.Context, db *pgxpool.Pool, cloudPath, assetID string, item metadata.Item, projections []OutputProjection) ([]OutputProjection, error) {
	var exists bool
	if db != nil {
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM strm_entries WHERE asset_id=$1)`, assetID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			rows, err := db.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, assetID)
			if err != nil {
				return nil, err
			}
			paths := []string{}
			for rows.Next() {
				var p string
				if err = rows.Scan(&p); err != nil {
					rows.Close()
					return nil, err
				}
				paths = append(paths, p)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			var season *int
			episodes := []int{}
			if item.MediaType == "series" {
				for _, p := range projections {
					s, _, ok := metadata.Episodes(p.Path)
					if !ok {
						return nil, errors.New("季集缺失")
					}
					season = &s
					episodes = append(episodes, p.SourceEpisodes...)
					if len(p.SourceEpisodes) == 0 {
						episodes = append(episodes, p.Episodes...)
					}
				}
				seen := map[int]bool{}
				unique := []int{}
				for _, n := range episodes {
					if !seen[n] {
						seen[n] = true
						unique = append(unique, n)
					}
				}
				episodes = unique
			}
			return ExistingOutputProjections(paths, item.MediaType, season, episodes)
		}
	}
	return applyTemplatePaths(ctx, db, cloudPath, assetID, item, projections)
}

func applyTemplatePaths(ctx context.Context, db *pgxpool.Pool, cloudPath, assetID string, item metadata.Item, projections []OutputProjection) ([]OutputProjection, error) {
	record, err := (settings.Repository{DB: db}).Get(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := (classification.Repository{DB: db}).Get(ctx)
	if err != nil {
		return nil, err
	}
	for i, p := range projections {
		season := 0
		if item.MediaType == "series" {
			var ok bool
			season, _, ok = metadata.Episodes(path.Base(p.Path))
			if !ok {
				return nil, errors.New("projection season unavailable")
			}
		}
		rendered, _, err := classification.Render(policy.Value, item, cloudPath, record.Value.MovieTemplate, record.Value.SeriesTemplate, ".strm", season, p.Episodes)
		if err != nil {
			return nil, err
		}
		projections[i].Path = rendered
	}
	return projections, nil
}

// Explicit migration/recovery compares the desired new geometry, not an
// existing legacy geometry adopted solely for normal idempotent import.
func DesiredOutputTemplates(ctx context.Context, db *pgxpool.Pool, cloudPath, assetID string, item metadata.Item, p []OutputProjection) ([]OutputProjection, error) {
	return applyTemplatePaths(ctx, db, cloudPath, assetID, item, p)
}
