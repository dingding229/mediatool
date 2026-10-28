package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/modules/nativelibrary"
	"slices"
	"strings"
)

// Only keys + user state participate in ranking. Do not materialize every
// episode's overview/still/runtime merely to pick one episode from each show.
var nextUpLightCTE = `,personal_series_ratings AS MATERIALIZED(
 SELECT b.id media_id,b.tmdb_id,coalesce(nullif(td.payload->>'officialRating',''),b.metadata->>'officialRating','') rating
 FROM (SELECT DISTINCT tmdb_id FROM keyed WHERE kind IN('series','season','episode')) root
 JOIN media_items b ON b.tmdb_id=root.tmdb_id AND b.media_type='series'
 LEFT JOIN native_tmdb_details td ON td.tmdb_type='series' AND td.tmdb_id=b.tmdb_id AND td.season_number IS NULL AND td.fetched_at IS NOT NULL
 AND td.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
),item_rows AS (
 SELECT g.kind,g.internal_key,n.id::text id,g.tmdb_id,g.s,g.e,g.libraries,
 coalesce(d.position_ticks,0) user_position,coalesce(d.play_count,0) user_count,coalesce(d.favorite,false) user_favorite,
 coalesce(d.played,false) user_played,d.last_played_at user_last_played,coalesce(o.overrides->>'officialRating',r.rating) rating
 FROM keyed g JOIN personal_series_ratings r ON r.tmdb_id=g.tmdb_id
 ` + metadataedit.Join("o", "r.media_id", "g.kind", "g.s", "g.e") + `
 LEFT JOIN LATERAL(SELECT id FROM native_item_ids WHERE entity_type=g.kind AND internal_key=g.internal_key LIMIT 1)n ON true
 LEFT JOIN media_user_item_data d ON d.item_id=n.id AND d.user_id=$3::uuid
 WHERE g.kind IN('series','season','episode')
) `

func (r BrowseRepository) nextUpPage(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery, series string, special bool) ([]dto.BaseItemDto, int, error) {
	visible, kind, parentKey, seriesKey, err := r.nextUpScope(ctx, u, p, q.ParentID, series)
	if err != nil {
		return nil, 0, err
	}
	where, args := conditions(nativelibrary.ItemQuery{IncludeItemTypes: []string{"Episode"}}, visible)
	addKey := func(key, ending string) {
		args = append(args, key+ending)
		where += fmt.Sprintf(" AND internal_key LIKE $%d::text", len(args))
	}
	if kind == "series" {
		addKey(parentKey, ":s%")
	}
	if kind == "season" {
		addKey(parentKey, "e%")
	}
	if seriesKey != "" {
		addKey(seriesKey, ":s%")
	}
	if !special {
		where += " AND s>0"
	}

	args = append(args, q.Limit, q.StartIndex)
	sql := strings.Replace(browseKeysCTE, "WHERE NOT l.hidden", "WHERE e.media_type='series' AND NOT l.hidden", 1) + nextUpLightCTE + nextUpCTE(where) + fmt.Sprintf(`,page AS(SELECT internal_key FROM filtered ORDER BY last_watch DESC,id::bigint NULLS LAST,internal_key LIMIT $%d OFFSET $%d)
 SELECT (SELECT count(*) FROM filtered),coalesce((SELECT jsonb_agg(internal_key) FROM page),'[]'),(SELECT count(*) FROM recent_shows)<=500`, len(args)-1, len(args))
	var total int
	var raw []byte
	var within bool
	if err = r.DB.QueryRow(ctx, sql, args...).Scan(&total, &raw, &within); err != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	if !within {
		return nil, 0, nativelibrary.ErrDiscoveryTooBroad
	}
	var keys []string
	if json.Unmarshal(raw, &keys) != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	if len(keys) == 0 {
		return []dto.BaseItemDto{}, total, nil
	}
	// Restrict BEFORE metadata/episode JSON expansion, not after a heavy join.
	pageSQL := strings.Replace(strings.Replace(strings.Replace(browseCTE, headerMetadata, fullPageMetadata+`||`+parentTextMetadata, 1), "WHERE NOT l.hidden", "WHERE e.media_type='series' AND NOT l.hidden", 1), "FROM keyed g JOIN media_items b", "FROM (SELECT * FROM keyed WHERE kind='episode' AND internal_key=ANY($4::text[])) g JOIN media_items b", 1)
	pageSQL += ` SELECT coalesce(jsonb_agg(row_to_json(page)),'[]') FROM (SELECT ` + browseColumns + ` FROM item_rows WHERE kind='episode')page`
	if err = r.DB.QueryRow(ctx, pageSQL, visible.ids, visible.library, visible.user, keys).Scan(&raw); err != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	var records []browseRow
	if json.Unmarshal(raw, &records) != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	items, err := r.hydrate(ctx, records, q.Fields, true)
	if err != nil {
		return nil, 0, err
	}
	byKey := map[string]dto.BaseItemDto{}
	for i, item := range items {
		if err = p.ItemAllowed(ctx, u, contracts.ItemAccess{ID: item.Id, LibraryIDs: records[i].Libraries, OfficialRating: records[i].Rating}); err != nil {
			return nil, 0, err
		}
		d := nativelibrary.CompleteUserData(records[i].UserData, item.Id, item.RunTimeTicks)
		item.UserData = &d
		byKey[records[i].Key] = item
	}
	out := make([]dto.BaseItemDto, 0, len(keys))
	for _, key := range keys {
		if item, ok := byKey[key]; ok {
			out = append(out, item)
		}
	}
	return out, total, nil
}

// Parent validation uses the SAME eligible keys and library scope but never
// hydrates a whole series/season merely to check existence for NextUp.
func (r BrowseRepository) nextUpScope(ctx context.Context, u contracts.User, p contracts.PermissionChecker, parent, series string) (s browseScope, kind, key, seriesKey string, err error) {
	s, err = scope(ctx, u, p)
	if err != nil {
		return
	}
	if r.DB == nil {
		err = contracts.ErrUnavailable
		return
	}
	ids := []string{}
	if parent != "" {
		ids = append(ids, parent)
	}
	if series != "" && series != parent {
		ids = append(ids, series)
	}
	if len(ids) == 0 {
		return
	}
	rows, e := r.DB.Query(ctx, `SELECT id::text,entity_type,internal_key FROM native_item_ids WHERE id::text=ANY($1::text[]) LIMIT 2`, ids)
	if e != nil {
		err = contracts.ErrUnavailable
		return
	}
	found := map[string][2]string{}
	for rows.Next() {
		var id, k, v string
		if e = rows.Scan(&id, &k, &v); e != nil {
			break
		}
		found[id] = [2]string{k, v}
	}
	e = errors.Join(e, rows.Err())
	rows.Close()
	if e != nil {
		err = contracts.ErrUnavailable
		return
	}
	wanted := []string{}
	if parent != "" {
		v, ok := found[parent]
		if !ok {
			err = nativelibrary.ErrNotFound
			return
		}
		kind, key = v[0], v[1]
		switch kind {
		case "library":
			if !slices.Contains(s.ids, key) {
				err = nativelibrary.ErrNotFound
				return
			}
			s.library = key
			var exists bool
			e = r.DB.QueryRow(ctx, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT EXISTS(SELECT 1 FROM native_libraries l WHERE l.id=$1 AND NOT l.hidden AND (l.id NOT IN('f:movie','f:series') OR EXISTS(SELECT 1 FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id WHERE al.library_id=l.id)))`, key).Scan(&exists)
			if e != nil {
				err = contracts.ErrUnavailable
				return
			}
			if !exists {
				err = nativelibrary.ErrNotFound
				return
			}
		case "series", "season":
			wanted = append(wanted, key)
		default:
			err = nativelibrary.ErrNotFound
			return
		}
	}
	if series != "" {
		v, ok := found[series]
		if !ok || v[0] != "series" {
			err = nativelibrary.ErrNotFound
			return
		}
		seriesKey = v[1]
		if !slices.Contains(wanted, seriesKey) {
			wanted = append(wanted, seriesKey)
		}
	}
	if len(wanted) == 0 {
		return
	}
	var count int
	e = r.DB.QueryRow(ctx, browseKeysCTE+nextUpLightCTE+` SELECT count(*) FROM item_rows WHERE internal_key=ANY($4::text[])`, s.ids, s.library, s.user, wanted).Scan(&count)
	if e != nil {
		err = contracts.ErrUnavailable
		return
	}
	if count != len(wanted) {
		err = nativelibrary.ErrNotFound
	}
	return
}
