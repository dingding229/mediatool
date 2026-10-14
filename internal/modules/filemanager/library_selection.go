package filemanager

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/releasename"
	"path"
	"strings"
)

type LibraryRow struct {
	Source     string   `json:"source"`
	Title      string   `json:"title"`
	Year       string   `json:"year"`
	MediaType  string   `json:"mediaType"`
	TMDBID     int      `json:"tmdbId"`
	Season     int      `json:"season"`
	Episode    int      `json:"episode"`
	Episodes   []int    `json:"episodes"`
	Resolution string   `json:"resolution"`
	Confirmed  bool     `json:"confirmed"`
	Warnings   []string `json:"warnings"`
}
type LibrarySelection struct {
	Root  Root         `json:"root"`
	Files []File       `json:"files"`
	Rows  []LibraryRow `json:"rows"`
}

// Selection comes from index IDs, not a caller-supplied absolute path. All
// selected files must share one enabled 115 connection; previews still verify
// live provider identity and all normal scan/transfer/reservation guards.
func (e Engine) LibrarySelection(ctx context.Context, candidates, assets []string) (LibrarySelection, error) {
	out := LibrarySelection{Files: []File{}, Rows: []LibraryRow{}}
	if len(candidates)+len(assets) < 1 || len(candidates)+len(assets) > 20 || len(candidates) > 0 && len(assets) > 0 {
		return out, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range append(candidates, assets...) {
		if len(id) != 36 || seen[id] {
			return out, ErrInvalid
		}
		seen[id] = true
	}
	rows, err := e.DB.Query(ctx, `SELECT f.storage_id::text,coalesce(f.provider_file_id,''),f.full_path,f.size_bytes,f.state,coalesce(m.title,''),coalesce(m.release_year,''),coalesce(m.media_type,''),coalesce(m.tmdb_id,0),coalesce(b.season,0),coalesce(b.episode,0),coalesce(b.episodes,'[]'::jsonb) FROM cloud_files f LEFT JOIN library_candidates c ON c.file_id=f.id LEFT JOIN media_assets a ON a.cloud_file_id=f.id LEFT JOIN media_asset_bindings b ON b.asset_id=a.id LEFT JOIN media_items m ON m.id=b.media_id WHERE (c.id::text=ANY($1::text[]) OR a.id::text=ANY($2::text[])) ORDER BY f.full_path`, candidates, assets)
	if err != nil {
		return out, ErrUnavailable
	}
	connection := ""
	for rows.Next() {
		var storage, id, full, state string
		var size int64
		var row LibraryRow
		var episodes []byte
		if err = rows.Scan(&storage, &id, &full, &size, &state, &row.Title, &row.Year, &row.MediaType, &row.TMDBID, &row.Season, &row.Episode, &episodes); err != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		if state != "present" || id == "" || connection != "" && storage != connection {
			rows.Close()
			return out, ErrProtected
		}
		connection = storage
		if json.Unmarshal(episodes, &row.Episodes) != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		hint := releasename.Parse(path.Base(full))
		row.Resolution = hint.Resolution
		row.Confirmed = row.TMDBID > 0
		row.Warnings = hint.Warnings
		if !row.Confirmed {
			row.Title, row.Year, row.MediaType, row.Season, row.Episode = hint.Title, hint.Year, hint.MediaType, hint.Season, hint.Episode
			row.Episodes = hint.Episodes
		}
		out.Files = append(out.Files, File{ID: id, Name: path.Base(full), Path: full, Size: size})
		out.Rows = append(out.Rows, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, ErrUnavailable
	}
	if len(out.Files) != len(seen) {
		return out, ErrMissing
	}
	root, err := e.root(ctx, connection)
	if err != nil {
		return out, err
	}
	if root.Kind != "cloud" || !(root.Path == "/115" || strings.HasPrefix(root.Path, "/115/")) {
		return out, ErrProtected
	}
	out.Root = root
	for i := range out.Files {
		f := &out.Files[i]
		prefix := strings.TrimRight(root.Path, "/") + "/"
		if !strings.HasPrefix(f.Path, prefix) {
			return out, ErrProtected
		}
		f.Path = strings.TrimPrefix(f.Path, prefix)
		if _, err := Relative(f.Path); err != nil {
			return out, err
		}
		out.Rows[i].Source = f.Path
	}
	return out, nil
}
