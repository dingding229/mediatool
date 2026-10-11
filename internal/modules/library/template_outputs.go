package library

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"path"
	"strings"
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
			return projections, nil
		}
	}
	record, err := (settings.Repository{DB: db}).Get(ctx)
	if err != nil {
		return nil, err
	}
	h := releasename.Parse(path.Base(cloudPath))
	h.Title, h.Year, h.MediaType, h.Extension = safeName(item.Title), item.Year, item.MediaType, ".strm"
	for i, p := range projections {
		template := record.Value.MovieTemplate
		if item.MediaType == "series" {
			template = record.Value.SeriesTemplate
			if len(p.Episodes) == 0 {
				return nil, errors.New("series coverage required")
			}
			s, ep, ok := metadata.Episodes(path.Base(p.Path))
			if !ok {
				return nil, errors.New("projection season unavailable")
			}
			h.Season, h.Episode = s, ep[0]
			tag, err := metadata.EpisodeTag(s, p.Episodes)
			if err != nil {
				return nil, err
			}
			template = strings.ReplaceAll(template, "S{season}E{episode}", tag)
		}
		name, err := releasename.Render(template, h, path.Base(cloudPath))
		if err != nil {
			return nil, err
		}
		// Keep stable edition scoping rather than collide different encodes of a title.
		if len(assetID) < 8 {
			return nil, errors.New("asset identity required")
		}
		name = strings.TrimSuffix(name, ".strm") + " [" + assetID[:8] + "].strm"
		if strings.ContainsAny(name, "/\\") || len(name) > 255 {
			return nil, errors.New("output name invalid")
		}
		projections[i].Path = path.Join(path.Dir(p.Path), name)
	}
	return projections, nil
}
