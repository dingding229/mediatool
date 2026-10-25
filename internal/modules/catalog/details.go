package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/metadataedit"
)

type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type ManagedDetail struct {
	Item
	Metadata    metadataedit.Values `json:"metadata"`
	Rating      *float64            `json:"rating"`
	Size        *int64              `json:"size"`
	Resolutions []Dimensions        `json:"resolutions"`
	Aggregate   bool                `json:"aggregate"`
	AssetCount  int                 `json:"assetCount"`
}

// Scope identities are the existing work/season/physical-asset identities.
func detailScope(id string) (string, []any, string, *int, error) {
	if uuidID.MatchString(id) {
		return "a.id=$1::uuid", []any{id}, "", nil, nil
	}
	parts := strings.Split(id, "_")
	if len(parts) < 2 || !uuidID.MatchString(parts[1]) {
		return "", nil, "", nil, ErrInvalid
	}
	where := "m.id=$1::uuid AND m.media_type='series'"
	args := []any{parts[1]}
	if parts[0] == "series" && len(parts) == 2 {
		return where, args, "Series", nil, nil
	}
	if parts[0] == "season" && len(parts) == 3 {
		n, e := strconv.Atoi(parts[2])
		if e == nil && n >= 0 && n <= 10000 {
			return where + " AND b.season=$2", append(args, n), "Season", &n, nil
		}
	}
	return "", nil, "", nil, ErrInvalid
}

// ManagedDetail reads persisted snapshots only. No provider/probe execution or
// protocol projection is involved; streams and storage links never leave here.
func (e Engine) ManagedDetail(parent context.Context, id string) (ManagedDetail, error) {
	out := ManagedDetail{Item: Item{ID: id, Source: "managed"}, Resolutions: []Dimensions{}}
	where, args, kind, season, err := detailScope(id)
	if err != nil {
		return out, err
	}
	if e.DB == nil {
		return out, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var basic, snapshot []byte
	var poster, mediaID string
	query := `SELECT coalesce(m.title,f.metadata->>'name',regexp_replace(f.full_path,'^.*/','','g')),coalesce(m.release_year,''),coalesce(m.media_type,'unknown'),coalesce(m.metadata,'{}'::jsonb),coalesce(d.payload,'{}'::jsonb),f.state,b.season,b.episode,coalesce(m.id::text,'')` + managedSnapshots + ` WHERE ` + where + ` ORDER BY a.id LIMIT 1`
	err = e.DB.QueryRow(ctx, query, args...).Scan(&out.Title, &out.Year, &out.MediaType, &basic, &snapshot, &out.State, &out.Season, &out.Episode, &mediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrInvalid
	}
	if err != nil {
		return out, ErrUnavailable
	}
	var item metadata.Item
	if json.Unmarshal(basic, &item) != nil {
		return out, ErrUnavailable
	}
	item.MediaType = out.MediaType
	out.Overview, poster = item.Overview, item.PosterPath
	var stored, cached struct {
		Rating   *float64 `json:"communityRating"`
		Overview string   `json:"overview"`
		Poster   string   `json:"posterPath"`
	}
	if json.Unmarshal(basic, &stored) == nil {
		out.Rating = stored.Rating
	}
	if json.Unmarshal(snapshot, &cached) == nil {
		if cached.Rating != nil {
			out.Rating = cached.Rating
		}
		if cached.Overview != "" {
			out.Overview = cached.Overview
		}
		if cached.Poster != "" {
			poster = cached.Poster
		}
	}
	if out.Rating != nil && (math.IsNaN(*out.Rating) || math.IsInf(*out.Rating, 0) || *out.Rating < 0 || *out.Rating > 10) {
		out.Rating = nil
	}
	if safePoster.MatchString(poster) {
		out.Poster = "/api/v1/media/tmdb/image?path=" + url.QueryEscape(poster)
	}
	policy, err := (classification.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return out, ErrUnavailable
	}
	out.CategoryID, out.Category = categoryFor(policy.Value, classificationInput(item, snapshot))
	out.Type = "Movie"
	if out.MediaType == "series" {
		out.Type = "Episode"
	}
	if kind != "" {
		out.Type = kind
		out.Aggregate = true
		out.State = ""
		out.Season = season
		out.Episode = nil
		if kind == "Season" {
			out.Title = "第 " + strconv.Itoa(*season) + " 季"
		}
	}
	// Each file contributes its size exactly once, not once per video stream.
	// Ignore malformed/unknown probe schemas; successful snapshots remain useful
	// while the next attempt is pending/failed, as in the native projection.
	technical := `WITH files AS(SELECT f.size_bytes,CASE WHEN p.probed_at IS NOT NULL AND coalesce(p.result->>'schema_version','0') IN('0','1') THEN p.result ELSE '{}'::jsonb END probe` + managedMembers + ` LEFT JOIN media_probe_results p ON p.cloud_file_id=f.id WHERE ` + where + `),dimensions AS(
 SELECT DISTINCT s->>'width' width,s->>'height' height FROM files CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(probe->'streams')='array' THEN probe->'streams' ELSE '[]'::jsonb END) s
 WHERE s->>'type'='Video' AND (s->>'width')~'^[0-9]{1,5}$' AND (s->>'height')~'^[0-9]{1,5}$' AND CASE WHEN (s->>'width')~'^[0-9]{1,5}$' THEN (s->>'width')::int BETWEEN 1 AND 32768 ELSE false END AND CASE WHEN (s->>'height')~'^[0-9]{1,5}$' THEN (s->>'height')::int BETWEEN 1 AND 32768 ELSE false END ORDER BY width,height LIMIT 64)
 SELECT (SELECT count(*) FROM files),(SELECT sum(size_bytes)::bigint FROM files),coalesce((SELECT jsonb_agg(jsonb_build_object('width',width::int,'height',height::int)) FROM dimensions),'[]'::jsonb)`
	var dims []byte
	if err = e.DB.QueryRow(ctx, technical, args...).Scan(&out.AssetCount, &out.Size, &dims); err != nil {
		return out, ErrUnavailable
	}
	if json.Unmarshal(dims, &out.Resolutions) != nil {
		return out, ErrUnavailable
	}
	// Use the same effective text as list/search and the native protocol. This
	// read never changes technical fields, classification, assets or STRM paths.
	record, editErr := (metadataedit.Repository{DB: e.DB}).Read(ctx, id)
	if editErr == nil {
		out.Metadata = record.Effective
		_ = json.Unmarshal(record.Effective["title"], &out.Title)
		_ = json.Unmarshal(record.Effective["overview"], &out.Overview)
		var y *int
		_ = json.Unmarshal(record.Effective["year"], &y)
		out.Year = ""
		if y != nil {
			out.Year = strconv.Itoa(*y)
		}
		out.Rating = nil
		_ = json.Unmarshal(record.Effective["communityRating"], &out.Rating)
	} else if !errors.Is(editErr, metadataedit.ErrNotFound) {
		return out, ErrUnavailable
	}
	return out, nil
}
