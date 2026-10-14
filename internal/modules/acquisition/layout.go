package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
)

func (e Engine) TransferProjections(ctx context.Context, transferID, cloudPath, asset string, item metadata.Item, season *int, episodes []int, desired ...bool) ([]library.OutputProjection, error) {
	projections, err := library.AssetOutputProjections(cloudPath, asset, item, season, episodes)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err = e.DB.QueryRow(ctx, `SELECT layout_snapshot FROM transfer_tasks WHERE id=$1`, transferID).Scan(&raw); err != nil {
		return nil, err
	}
	var layout struct {
		MovieTemplate  string                `json:"movieTemplate"`
		SeriesTemplate string                `json:"seriesTemplate"`
		Classification classification.Policy `json:"classification"`
		Source         string                `json:"source"`
		Media          metadata.Item         `json:"media"`
	}
	if json.Unmarshal(raw, &layout) != nil {
		return nil, errors.New("上传布局快照损坏")
	}
	if layout.MovieTemplate == "" {
		if len(desired) > 0 && desired[0] {
			return library.DesiredOutputTemplates(ctx, e.DB, cloudPath, asset, item, projections)
		}
		return library.ApplyOutputTemplates(ctx, e.DB, cloudPath, asset, item, projections)
	}
	if layout.Media.ID != item.ID || layout.Media.MediaType != item.MediaType {
		return nil, errors.New("上传布局与当前媒体身份不一致，须明确迁移")
	}
	for index, p := range projections {
		s := 0
		if season != nil {
			s = *season
		}
		name, _, err := classification.Render(layout.Classification, layout.Media, layout.Source, layout.MovieTemplate, layout.SeriesTemplate, ".strm", s, p.Episodes)
		if err != nil {
			return nil, err
		}
		projections[index].Path = name
	}
	return projections, nil
}
