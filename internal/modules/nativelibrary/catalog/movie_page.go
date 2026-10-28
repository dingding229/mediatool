package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/modules/nativelibrary"
	"strings"
)

// Movie cards sorted by metadata/user data need only an eligibility existence
// probe before paging, not aggregation of every physical version. Date-created
// ranking retains the generic path because it depends on version timestamps.
func (r BrowseRepository) moviePage(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery, o browseOptions, s browseScope, where string, args []any, order []string) ([]dto.BaseItemDto, int, bool, error) {
	if len(q.IncludeItemTypes) != 1 || !strings.EqualFold(q.IncludeItemTypes[0], "Movie") || len(q.IDs) > 0 || o.search != "" || q.SearchTerm != "" || s.parentKind != "" && s.parentKind != "library" {
		return nil, 0, false, nil
	}
	for _, sort := range order {
		if strings.Contains(sort, "created") {
			return nil, 0, false, nil
		}
	}
	args = append([]any(nil), args...)
	if o.extra != nil {
		where, args = o.extra(where, args)
	}
	rootFilter := ""

	cols, ordering, err := projectionOrder(order)
	if err != nil {
		return nil, 0, true, err
	}
	cap := ""
	within := "true"
	if o.bounded {
		cap = fmt.Sprintf(" LIMIT %d", discoveryCandidateLimit+1)
		within = fmt.Sprintf("(SELECT count(*)FROM filtered)<=%d", discoveryCandidateLimit)
	}
	sql := `WITH eligible_movies AS MATERIALIZED(
 SELECT DISTINCT e.media_id FROM (` + ExposableAssetsSQL + `)e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 WHERE e.media_type='movie' ` + rootFilter + ` AND NOT l.hidden AND l.collection_type='movies' AND l.id=ANY($1::text[]) AND ($2::text='' OR l.id=$2)
 ),roots AS NOT MATERIALIZED(
 SELECT m.id media_id,m.tmdb_id,m.title,m.release_year,m.metadata,n.id::text id FROM eligible_movies k JOIN media_items m ON m.id=k.media_id
 LEFT JOIN LATERAL(SELECT id FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:'||m.tmdb_id LIMIT 1)n ON true
 ),headers AS NOT MATERIALIZED(
 SELECT r.*,(` + metadataedit.HeaderSQL("r.title", "r.release_year", "r.metadata", "'movie'", "-1", "-1", "td.payload", "'{}'::jsonb", "'{}'::jsonb", "o.overrides") + `) meta
 FROM roots r LEFT JOIN native_tmdb_details td ON td.tmdb_type='movie' AND td.tmdb_id=r.tmdb_id AND td.season_number IS NULL AND td.fetched_at IS NOT NULL
 AND td.language=coalesce((SELECT nullif(settings->>'language','')FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 ` + metadataedit.HeaderJoin("o", "r.media_id", "'movie'", "-1", "-1") + `
 ),item_rows AS NOT MATERIALIZED(
 SELECT 'movie'::text kind,'tmdb:'||h.tmdb_id internal_key,h.id,-1 s,-1 e,coalesce(h.meta->>'title',h.title) name,CASE WHEN h.meta?'year' THEN coalesce(h.meta->>'year','') ELSE h.release_year END item_year,h.meta,coalesce(h.meta->>'officialRating','') rating,coalesce(h.meta->>'premiereDate','') premiere,
 coalesce(d.played,false)user_played,coalesce(d.position_ticks,0)user_position,coalesce(d.play_count,0)user_count,coalesce(d.favorite,false)user_favorite,d.last_played_at user_last_played
 FROM headers h LEFT JOIN media_user_item_data d ON d.user_id=$3::uuid AND d.item_id=h.id::bigint
 ),filtered AS MATERIALIZED(SELECT ` + strings.Join(cols, ",") + ` FROM item_rows WHERE ` + where + cap + `)`
	source, offset, anchor := "filtered", "", "true"
	if o.startID != "" {
		args = append(args, o.startID)
		n := len(args)
		sql += ",numbered AS(SELECT *,row_number() OVER(ORDER BY " + strings.Join(ordering, ",") + ")-1 ordinal FROM filtered)"
		source = "numbered"
		offset = fmt.Sprintf("coalesce((SELECT ordinal FROM numbered WHERE id=$%d::text),0)+", n)
		anchor = fmt.Sprintf("EXISTS(SELECT 1 FROM filtered WHERE id=$%d::text)", n)
	}
	args = append(args, q.Limit, q.StartIndex)
	sql += ",page AS(SELECT id,internal_key FROM " + source + " ORDER BY " + strings.Join(ordering, ",") + fmt.Sprintf(" LIMIT $%d OFFSET "+offset+"$%d)", len(args)-1, len(args)) + "SELECT(SELECT count(*)FROM filtered),coalesce((SELECT jsonb_agg(row_to_json(page))FROM page),'[]')," + within + "," + anchor
	var total int
	var raw []byte
	var bounded, found bool
	if err = r.DB.QueryRow(ctx, sql, args...).Scan(&total, &raw, &bounded, &found); err != nil {
		return nil, 0, true, contracts.ErrUnavailable
	}
	if !bounded {
		return nil, 0, true, nativelibrary.ErrDiscoveryTooBroad
	}
	if !found {
		return nil, 0, true, nativelibrary.ErrNotFound
	}
	var keys []struct {
		ID  *string `json:"id"`
		Key string  `json:"internal_key"`
	}
	if json.Unmarshal(raw, &keys) != nil {
		return nil, 0, true, contracts.ErrUnavailable
	}
	if len(keys) == 0 {
		return []dto.BaseItemDto{}, total, true, nil
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		if key.ID == nil {
			return nil, 0, false, nil
		}
		ids = append(ids, *key.ID)
	}
	page := q
	page.IDs = ids
	page.StartIndex = 0
	page.Limit = len(ids)
	o.startID = ""
	o.bounded = false
	o.skipCount = true
	items, _, err := r.query(ctx, u, p, page, o)
	if err != nil {
		return nil, 0, true, err
	}
	byID := map[string]dto.BaseItemDto{}
	for _, item := range items {
		byID[item.Id] = item
	}
	out := make([]dto.BaseItemDto, 0, len(items))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out, total, true, nil
}
