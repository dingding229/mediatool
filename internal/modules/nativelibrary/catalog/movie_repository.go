package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativelibrary"
)

// Internal service only; T17 must authorize the library before invoking Query.
// Requiring one library makes ParentId unambiguous for global Movie IDs.
type MovieQuery struct {
	Visible           nativelibrary.VisibleLibraries
	StartIndex, Limit int
	LibraryID, SortBy string
	Descending        bool
}
type MovieRepository struct{ DB *pgxpool.Pool }

var movieSorts = map[string]string{
	"SortName":        `lower(coalesce(nullif(p.metadata->>'sortName',''),p.title)) COLLATE "C"`,
	"DateCreated":     `p.date_created`,
	"ProductionYear":  `CASE WHEN p.release_year~'^[0-9]{4}$' THEN p.release_year::int ELSE NULL END`,
	"CommunityRating": `CASE WHEN jsonb_typeof(coalesce(d.payload->'communityRating',p.metadata->'communityRating'))='number' THEN (coalesce(d.payload->>'communityRating',p.metadata->>'communityRating'))::numeric ELSE NULL END`,
}

func (r MovieRepository) Query(ctx context.Context, q MovieQuery) ([]dto.BaseItemDto, int, error) {
	if q.SortBy == "" {
		q.SortBy = "SortName"
	}
	order, ok := movieSorts[q.SortBy]
	if !ok || q.LibraryID == "" || q.StartIndex < 0 || q.StartIndex > 1000000 || q.Limit < 1 || q.Limit > 200 {
		return nil, 0, ErrInvalid
	}
	if !q.Visible.Valid() || q.Visible.RequestedID != q.LibraryID {
		return nil, 0, nativelibrary.ErrNotFound
	}
	if r.DB == nil {
		return nil, 0, nativelibrary.ErrSettingsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	direction := " ASC"
	if q.Descending {
		direction = " DESC"
	}
	cte := `WITH eligible AS (` + ExposableAssetsSQL + `), members AS(
 SELECT e.* FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id
 JOIN native_libraries l ON l.id=al.library_id
 WHERE e.media_type='movie' AND l.id=$1 AND NOT l.hidden), movies AS(
 SELECT media_id,tmdb_id,title,release_year,metadata,min(created_at) AS date_created
 FROM members GROUP BY media_id,tmdb_id,title,release_year,metadata) `
	var total int
	if err := r.DB.QueryRow(ctx, cte+`SELECT count(*) FROM movies`, q.LibraryID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, cte+`SELECT p.tmdb_id,p.title,p.release_year,p.metadata,p.date_created,l.item_id::text,trim(n.server_id::text),
 (SELECT coalesce(jsonb_agg(jsonb_build_object('ID',s.asset_id::text,'Container',s.container) ORDER BY s.asset_id),'[]') FROM (
 SELECT m.asset_id,coalesce(lower(substring(f.full_path FROM '\.([A-Za-z0-9]{1,16})$')),'') AS container
 FROM members m JOIN cloud_files f ON f.id=m.cloud_file_id WHERE m.media_id=p.media_id ORDER BY m.asset_id LIMIT 101) s),coalesce(d.payload,'{}'::jsonb)
 FROM movies p JOIN native_libraries l ON l.id=$1 CROSS JOIN native_server_settings n
 LEFT JOIN native_tmdb_details d ON d.tmdb_type='movie' AND d.tmdb_id=p.tmdb_id AND d.season_number IS NULL AND d.episode_number IS NULL AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 ORDER BY `+order+direction+` NULLS LAST,tmdb_id ASC LIMIT $2 OFFSET $3`, q.LibraryID, q.Limit, q.StartIndex)
	if err != nil {
		return nil, 0, err
	}
	type record struct {
		tmdbID                          int64
		title, year, parentID, serverID string
		metadata, details               []byte
		created                         time.Time
		sources                         []nativelibrary.MovieSource
	}
	records := []record{}
	keys := []nativelibrary.Key{}
	for rows.Next() {
		var row record
		var sources []byte
		if err := rows.Scan(&row.tmdbID, &row.title, &row.year, &row.metadata, &row.created, &row.parentID, &row.serverID, &sources, &row.details); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if err := json.Unmarshal(sources, &row.sources); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if len(row.sources) > 100 {
			rows.Close()
			return nil, 0, errors.New("movie has more than 100 sources")
		}
		key, err := nativelibrary.MovieKey(row.tmdbID)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		keys = append(keys, key)
		records = append(records, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	var pageSources []*nativelibrary.MovieSource
	for _, row := range records {
		for i := range row.sources {
			pageSources = append(pageSources, &row.sources[i])
		}
	}
	if err := hydrateProbeSources(ctx, r.DB, pageSources); err != nil {
		return nil, 0, err
	}
	ids, err := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, keys)
	if err != nil {
		return nil, 0, err
	}
	out := []dto.BaseItemDto{}
	for i, row := range records {
		var meta nativelibrary.StoredMovieMetadata
		if err := json.Unmarshal(row.metadata, &meta); err != nil {
			return nil, 0, err
		}
		movie := nativelibrary.Movie{ID: strconv.FormatInt(ids[keys[i]], 10), ServerID: row.serverID, ParentID: row.parentID, Name: row.title, Year: row.year, TMDBID: row.tmdbID, Metadata: meta, CreatedAt: row.created, Sources: row.sources}
		var details nativelibrary.TMDBDetails
		if json.Unmarshal(row.details, &details) == nil {
			movie.Details = &details
		}
		item, err := nativelibrary.MovieDTO(movie)
		if err != nil && movie.Details != nil {
			movie.Details = nil
			item, err = nativelibrary.MovieDTO(movie)
		}
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, nil
}
