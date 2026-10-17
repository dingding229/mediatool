package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

// Taxonomy IDs never enter the five content-entity sequence or playback routes.
func GenreID(canonical string) string {
	digest := sha256.Sum256([]byte(canonical))
	return "genre:" + hex.EncodeToString(digest[:])
}
func (r BrowseRepository) facets(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery) (nativelibrary.DiscoveryResult, error) {
	out := nativelibrary.DiscoveryResult{Items: []dto.BaseItemDto{}, Genres: []dto.NameIdPair{}, Filters: dto.ItemFilters{Genres: dto.Array[string]{}, Years: dto.Array[int]{}, OfficialRatings: dto.Array[dto.NameValuePair]{}}}
	if len(q.IncludeItemTypes) == 0 {
		q.IncludeItemTypes = []string{"Movie", "Series", "Episode"}
	}
	_, where, args, err := r.plan(ctx, u, p, q)
	if err != nil {
		return out, err
	}
	searchBind := 0
	if q.SearchTerm != "" {
		args = append(args, q.SearchTerm)
		searchBind = len(args)
	}
	sql := browseSearchSQL(q, searchBind, false) + fmt.Sprintf(`,candidates AS MATERIALIZED(SELECT * FROM item_rows WHERE %s LIMIT %d),facet_values AS(
 SELECT 'genre' kind,lower(normalize(btrim(v),NFKC)) key,min(v COLLATE "C") name FROM candidates c
 CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(c.meta->'genres')='array' THEN c.meta->'genres' ELSE '[]'::jsonb END) v WHERE btrim(v)<>'' GROUP BY lower(normalize(btrim(v),NFKC))
 UNION ALL SELECT 'year',item_year,item_year FROM candidates WHERE item_year~'^[0-9]{4}$' GROUP BY item_year
 UNION ALL SELECT 'rating',rating,rating FROM candidates WHERE rating<>'' GROUP BY rating
 ),ranked AS(SELECT *,row_number() OVER(PARTITION BY kind ORDER BY key COLLATE "C") rank FROM facet_values)
 SELECT (SELECT count(*) FROM candidates),coalesce((SELECT jsonb_agg(row_to_json(ranked) ORDER BY kind,key COLLATE "C") FROM ranked WHERE rank<=513),'[]')`, where, discoveryCandidateLimit+1)
	var count int
	var raw []byte
	if err := r.DB.QueryRow(ctx, sql, args...).Scan(&count, &raw); err != nil {
		return out, contracts.ErrUnavailable
	}
	if count > discoveryCandidateLimit {
		return out, nativelibrary.ErrDiscoveryTooBroad
	}
	var values []struct{ Kind, Key, Name string }
	if err := json.Unmarshal(raw, &values); err != nil {
		return out, contracts.ErrUnavailable
	}
	for _, v := range values {
		switch v.Kind {
		case "genre":
			out.Genres = append(out.Genres, dto.NameIdPair{Name: v.Name, Id: GenreID(v.Key)})
			out.Filters.Genres = append(out.Filters.Genres, v.Name)
		case "year":
			year, err := strconv.Atoi(v.Key)
			if err != nil {
				return out, contracts.ErrUnavailable
			}
			out.Filters.Years = append(out.Filters.Years, year)
		case "rating":
			out.Filters.OfficialRatings = append(out.Filters.OfficialRatings, dto.NameValuePair{Name: v.Name, Value: v.Key})
		}
	}
	if len(out.Genres) > 512 || len(out.Filters.Years) > 512 || len(out.Filters.OfficialRatings) > 128 {
		return out, nativelibrary.ErrDiscoveryTooBroad
	}
	out.Total = len(out.Genres)
	return out, nil
}
