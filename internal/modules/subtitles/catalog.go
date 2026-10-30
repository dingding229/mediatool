package subtitles

import (
	"context"
	uuidcheck "media-tool/internal/platform/identity"
	"media-tool/internal/platform/mediaext"
	"strconv"
	"strings"
)

type CatalogPage struct {
	SelectedAssetID string    `json:"selectedAssetId"`
	Versions        []Version `json:"versions"`
	Groups          []Page    `json:"groups"`
}

// Catalog lists version choices without reading provider files. A series/season
// requires an explicit physical episode selection before listing its groups.
func (r Repository) Catalog(ctx context.Context, id, selected string) (CatalogPage, error) {
	out := CatalogPage{Versions: []Version{}, Groups: []Page{}}
	if r.DB == nil {
		return out, ErrUnavailable
	}
	var work string
	var season, episode *int
	if uuidcheck.UUID(id) {
		if e := r.asset(ctx, id); e != nil {
			return out, e
		}
		if e := r.DB.QueryRow(ctx, `SELECT media_id::text,season,episode FROM media_asset_bindings WHERE asset_id=$1`, id).Scan(&work, &season, &episode); e != nil {
			return out, ErrNotFound
		}
		selectedDefault := id
		if selected == "" {
			selected = selectedDefault
		}
	} else {
		parts := strings.Split(id, "_")
		if len(parts) < 2 || !uuidcheck.UUID(parts[1]) {
			return out, ErrInvalid
		}
		work = parts[1]
		if parts[0] == "season" && len(parts) == 3 {
			n, e := strconv.Atoi(parts[2])
			if e != nil || n < 0 || n > 10000 {
				return out, ErrInvalid
			}
			season = &n
		} else if parts[0] != "series" || len(parts) != 2 {
			return out, ErrInvalid
		}
	}
	rows, e := r.DB.Query(ctx, `SELECT a.id::text,regexp_replace(f.full_path,'^.*/',''),b.season,b.episode FROM media_asset_bindings b JOIN media_assets a ON a.id=b.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id JOIN media_items m ON m.id=b.media_id WHERE b.media_id=$1 AND ($2::int IS NULL OR b.season=$2) AND ($3::int IS NULL OR b.episode=$3) AND (m.media_type='movie' OR m.media_type='series' AND b.episode IS NOT NULL) ORDER BY b.season NULLS FIRST,b.episode NULLS FIRST,a.id LIMIT 1001`, work, season, episode)
	if e != nil {
		return out, ErrUnavailable
	}
	for rows.Next() {
		var v Version
		if e = rows.Scan(&v.ID, &v.Name, &v.Season, &v.Episode); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		out.Versions = append(out.Versions, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	if len(out.Versions) > 1000 {
		return out, ErrInvalid
	}
	if selected == "" {
		return out, nil
	}
	var chosen *Version
	for i := range out.Versions {
		if out.Versions[i].ID == selected {
			chosen = &out.Versions[i]
			break
		}
	}
	if chosen == nil {
		return out, ErrInvalid
	}
	out.SelectedAssetID = selected
	ids := []string{}
	for _, v := range out.Versions {
		same := v.Season == nil && chosen.Season == nil || v.Season != nil && chosen.Season != nil && *v.Season == *chosen.Season
		same = same && (v.Episode == nil && chosen.Episode == nil || v.Episode != nil && chosen.Episode != nil && *v.Episode == *chosen.Episode)
		if same {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) > 100 {
		return out, ErrInvalid
	}
	groups, e := r.Groups(ctx, ids, mediaext.SubtitleExtensions())
	if e != nil {
		return out, e
	}
	for _, asset := range ids {
		items := groups[asset]
		if items == nil {
			items = []Entry{}
		}
		out.Groups = append(out.Groups, Page{AssetID: asset, Versions: []Version{}, Items: items})
	}
	return out, nil
}
