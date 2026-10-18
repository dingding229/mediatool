package nativetmdb

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/nativelibrary"
)

// Stored returns a parsed provider snapshot, never a TMDB client/token or raw
// payload. It does not fetch or schedule missing details.
func (r Repository) Stored(ctx context.Context, t Target, language string) (nativelibrary.TMDBDetails, string, error) {
	var out nativelibrary.TMDBDetails
	if !t.Valid() || language == "" || len(language) > 16 {
		return out, "", ErrInvalid
	}
	var raw []byte
	var state string
	err := r.DB.QueryRow(ctx, `SELECT payload,state FROM native_tmdb_details WHERE tmdb_type=$1 AND tmdb_id=$2 AND season_number IS NOT DISTINCT FROM $3::integer AND language=$4`, t.Type, t.TMDBID, t.Season, language).Scan(&raw, &state)
	if err != nil {
		return out, "", err
	}
	if len(raw) > 512<<10 {
		return out, "", ErrInvalid
	}
	err = json.Unmarshal(raw, &out)
	return out, state, err
}
