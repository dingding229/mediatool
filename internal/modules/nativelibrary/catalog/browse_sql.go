package catalog

import (
	"fmt"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/modules/nativelibrary"
	"strings"
)

// Scope is applied before grouping: a library ParentId must never borrow
// versions, episodes, dates or counts from another visible library.
const browseExpandedCTE = `WITH eligible AS NOT MATERIALIZED (` + ExposableAssetsSQL + `), bound AS NOT MATERIALIZED (
 SELECT e.*,al.library_id,l.item_id::text library_item,l.sort_order,
 CASE WHEN e.media_type='series' THEN coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(e.episodes) WHERE value~'^[0-9]+$'),e.episode) END first_episode
 FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 WHERE NOT l.hidden AND l.id=ANY($1::text[]) AND ($2::text='' OR l.id=$2)
 AND e.media_type=CASE l.collection_type WHEN 'movies' THEN 'movie' ELSE 'series' END
), members AS NOT MATERIALIZED (
 SELECT * FROM bound WHERE media_type='movie' OR season>=0 AND first_episode>0
), expanded AS NOT MATERIALIZED (
 SELECT m.*,k.kind,k.s,k.e FROM members m CROSS JOIN LATERAL (VALUES
 ('movie',-1,-1),('series',-1,-1),('season',m.season,-1),('episode',m.season,m.first_episode)) k(kind,s,e)
 WHERE (m.media_type='movie' AND k.kind='movie') OR (m.media_type='series' AND k.kind<>'movie')
) `
const browseKeysCTE = browseExpandedCTE + `,grouped AS (
 SELECT kind,tmdb_id,s,e,min(created_at) created,max(created_at) latest_created,array_agg(DISTINCT library_id) libraries,
 (array_agg(library_item ORDER BY sort_order,library_id))[1] parent,
 count(DISTINCT season)::int seasons,count(DISTINCT (season,first_episode))::int episodes
 FROM expanded GROUP BY kind,tmdb_id,s,e
), keyed AS (
 SELECT g.*,'tmdb:'||tmdb_id||CASE WHEN kind IN('season','episode') THEN ':s'||s ELSE '' END||CASE WHEN kind='episode' THEN 'e'||e ELSE '' END internal_key FROM grouped g
) `

// media_items has UNIQUE(media_type,tmdb_id). Joining its indexed root row
// avoids a second derived-table join whose underestimated cardinality caused
// quadratic nested-loop scans on freshly bulk-populated test libraries.
// Only lightweight headers participate in filtering and paging. Provider
// snapshots and physical versions are fetched once, only for this page's keys.
const parentTextMetadata = `jsonb_build_object('_seriesName',coalesce(ro.overrides->>'title',b.title),'_seasonName',coalesce(so.overrides->>'title',nullif(sd.payload->>'name',''),CASE WHEN g.s=0 THEN '特别篇' ELSE '第 '||g.s||' 季' END))`

var fullPageMetadata = metadataedit.BaseSQL("b.title", "b.release_year", "b.metadata", "g.kind", "g.s", "g.e", "d.payload", "sd.payload", "ep.value") + `||coalesce(o.overrides,'{}'::jsonb)`
var headerMetadata = metadataedit.HeaderSQL("b.title", "b.release_year", "b.metadata", "g.kind", "g.s", "g.e", "d.payload", "sd.payload", "ep.value", "o.overrides")
var browseCTE = browseKeysCTE + `,episode_metadata AS MATERIALIZED (
 SELECT DISTINCT ON(m.tmdb_id,m.season,m.first_episode) m.tmdb_id,m.season,m.first_episode,v.value
 FROM members m CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(m.asset_identity->'episodeDetails')='array' THEN m.asset_identity->'episodeDetails' ELSE '[]'::jsonb END) v(value)
 WHERE m.media_type='series' AND v.value->>'season'=m.season::text AND v.value->>'number'=m.first_episode::text
 ORDER BY m.tmdb_id,m.season,m.first_episode,m.created_at DESC,m.asset_id
),headers AS NOT MATERIALIZED (
 SELECT g.*,b.title,b.release_year,coalesce(nullif(sd.payload->>'name',''),CASE WHEN g.s=0 THEN '特别篇' ELSE '第 '||g.s||' 季' END) season_name,coalesce(sd.payload->>'premiereDate','') season_date,
 (` + headerMetadata + `) meta,
 coalesce(ep.value,'{}'::jsonb) episode_info
 FROM keyed g JOIN media_items b ON b.tmdb_id=g.tmdb_id AND b.media_type=CASE WHEN g.kind='movie' THEN 'movie' ELSE 'series' END
 LEFT JOIN native_tmdb_details d ON d.tmdb_type=b.media_type AND d.tmdb_id=g.tmdb_id AND d.season_number IS NULL AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=g.tmdb_id AND sd.season_number=g.s AND sd.fetched_at IS NOT NULL
 AND sd.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 LEFT JOIN episode_metadata em ON g.kind='episode' AND em.tmdb_id=g.tmdb_id AND em.season=g.s AND em.first_episode=g.e
 LEFT JOIN LATERAL(SELECT CASE WHEN g.kind='episode' THEN ` + metadataedit.LeafSQL("sd.payload", "jsonb_build_object('episodeDetails',jsonb_build_array(em.value))", "g.s", "g.e") + ` ELSE '{}'::jsonb END value) ep ON true
 ` + metadataedit.HeaderJoin("o", "b.id", "g.kind", "g.s", "g.e") + metadataedit.Join("ro", "b.id", "b.media_type", "-1", "-1") + metadataedit.Join("so", "b.id", "'season'", "g.s", "-1") + `
), media_rows AS NOT MATERIALIZED (
 SELECT h.kind,h.internal_key,h.tmdb_id,h.s,h.e,h.created,h.libraries,h.parent,h.title,h.meta,h.episode_info,h.seasons,h.episodes,n.id::text id,
 coalesce(meta->>'title',CASE kind WHEN 'season' THEN season_name WHEN 'episode' THEN coalesce(nullif(episode_info->>'name',''),nullif(episode_info->>'title',''),'第 '||e||' 集') ELSE title END) name,
 CASE WHEN meta?'year' THEN coalesce(meta->>'year','') WHEN kind='season' THEN coalesce(nullif(left(season_date,4),''),release_year) WHEN kind='episode' THEN coalesce(nullif(left(episode_info->>'airDate',4),''),release_year) ELSE release_year END AS item_year,
 coalesce(meta->>'officialRating','') rating,coalesce(meta->>'premiereDate','') premiere,0 sort_order,'' collection_type,h.latest_created
 FROM headers h LEFT JOIN LATERAL(SELECT id FROM native_item_ids WHERE entity_type=h.kind AND internal_key=h.internal_key LIMIT 1)n ON true
), raw_item_rows AS NOT MATERIALIZED (
 SELECT * FROM media_rows UNION ALL
 SELECT 'library',l.id,0::bigint,-1,-1,l.created_at,ARRAY[l.id],'',l.name,'{}'::jsonb,'{}'::jsonb,0,0,l.item_id::text,l.name,'','','',l.sort_order,l.collection_type,l.created_at
 FROM native_libraries l WHERE l.id=ANY($1::text[]) AND NOT l.hidden AND ($2::text='' OR l.id=$2)
 AND (l.id NOT IN('f:movie','f:series') OR EXISTS(SELECT 1 FROM members m WHERE m.library_id=l.id))
) ` + personalCTE

const browseColumns = `kind,internal_key AS key,tmdb_id AS tmdb,s,e,created,libraries,parent,title,name,item_year AS "Year",rating,
 meta||jsonb_build_object('overview',left(coalesce(meta->>'overview',''),16384)) AS meta,
 episode_info||jsonb_build_object('overview',left(coalesce(episode_info->>'overview',''),16384)) AS episode_info,seasons,episodes,collection_type,
 jsonb_strip_nulls(jsonb_build_object('Played',user_played,'PlaybackPositionTicks',user_position,'PlayCount',user_count,'IsFavorite',user_favorite,'LastPlayedDate',user_last_played,'UnplayedItemCount',user_unplayed,'PlayedPercentage',user_percentage)) AS user_data`

var browseSorts = map[string]string{
	"Name": "lower(name) COLLATE \"C\"", "SortName": "lower(coalesce(nullif(meta->>'sortName',''),name)) COLLATE \"C\"",
	"DateLastContentAdded": "latest_created",
	"DateCreated":          "created", "DatePlayed": "user_last_played", "PremiereDate": "NULLIF(premiere,'')", "ProductionYear": "CASE WHEN item_year~'^[0-9]{4}$' THEN item_year::int END",
	"CommunityRating": "CASE WHEN jsonb_typeof(meta->'communityRating')='number' THEN (meta->>'communityRating')::numeric END",
	"IndexNumber":     "CASE WHEN kind='season' THEN s ELSE e END", "ParentIndexNumber": "s", "IsFolder": "kind NOT IN('movie','episode')",
	"OfficialRating": "rating", "PlayCount": "user_count", "Runtime": "(0)",
}

// A movie-only request cannot need episode-derived data. Push this constraint
// before the shared CTE's materialization, rather than grouping invisible TV
// entities and discarding them after ranking. No new index/extension required.
func browseSQLFor(q nativelibrary.ItemQuery) string {
	if len(q.IncludeItemTypes) == 1 && strings.EqualFold(q.IncludeItemTypes[0], "Movie") {
		sql := strings.Replace(browseCTE, "WHERE NOT l.hidden", "WHERE e.media_type='movie' AND NOT l.hidden", 1)
		return strings.Replace(sql, "bound AS NOT MATERIALIZED", "bound AS MATERIALIZED", 1)
	}
	return browseCTE
}

// Search pruning is a safe superset for television (episode/season names are
// evaluated later) and exact for movies. It avoids sorting/materializing an
// entire movie library to answer a selective normalized title hint.
func browseSearchSQL(q nativelibrary.ItemQuery, bind int, normalized bool) string {
	sql := browseSQLFor(q)
	if bind == 0 {
		return sql
	}
	title := `(coalesce((SELECT overrides->>'title' FROM media_metadata_overrides WHERE media_id=e.media_id AND entity_type='movie' AND season_number=-1 AND episode_number=-1),e.title))`
	expr := fmt.Sprintf("strpos(lower(%s),lower($%d::text))>0", title, bind)
	if normalized {
		// Skip 100k empty override probes when this library has no fetched movie
		// original titles. When overrides exist, keep the indexed correlated lookup
		// (not an unindexed materialized title table that can become quadratic).
		language := `coalesce((SELECT nullif(settings->>'language','')FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')`
		sql = strings.Replace(sql, "WITH ", `WITH movie_snapshot_presence AS MATERIALIZED(SELECT EXISTS(SELECT 1 FROM native_tmdb_details WHERE tmdb_type='movie' AND season_number IS NULL AND fetched_at IS NOT NULL AND language=`+language+` AND nullif(payload->>'originalTitle','') IS NOT NULL) present), `, 1)
		original := `coalesce(CASE WHEN (SELECT present FROM movie_snapshot_presence) THEN (SELECT nullif(d.payload->>'originalTitle','') FROM native_tmdb_details d WHERE d.tmdb_type='movie' AND d.tmdb_id=e.tmdb_id AND d.season_number IS NULL AND d.fetched_at IS NOT NULL AND d.language=` + language + ` LIMIT 1)END,e.metadata->>'originalTitle','')`
		original = `(coalesce((SELECT overrides->>'originalTitle' FROM media_metadata_overrides WHERE media_id=e.media_id AND entity_type='movie' AND season_number=-1 AND episode_number=-1),` + original + `))`
		expr = fmt.Sprintf("strpos(lower(normalize(%s,NFKC)),lower(normalize($%d::text,NFKC)))>0 OR strpos(lower(normalize(%s,NFKC)),lower(normalize($%d::text,NFKC)))>0", title, bind, original, bind)
	}

	return strings.Replace(sql, "AND e.media_type=CASE l.collection_type", "AND (e.media_type<>'movie' OR ("+expr+")) AND e.media_type=CASE l.collection_type", 1)
}

// Restrict the physical members before materialized grouping. Keep outer parent
// checks for visibility and cross-series rejection; bind every client value.
func pruneParentSQL(sql string, bind int, kinds ...string) string {
	if bind == 0 {
		return sql
	}
	key := fmt.Sprintf(`(SELECT internal_key FROM native_item_ids WHERE id=$%d::bigint AND entity_type IN('series','season'))`, bind)
	tmdb := `split_part(substring(` + key + ` from 6),':',1)`
	kind := ""
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	if kind == "library" {
		return sql
	}
	condition := ` AND (` + key + ` IS NULL OR (m.media_type='series' AND m.tmdb_id=(` + tmdb + `)::bigint AND (` + key + ` NOT LIKE '%:s%' OR b.season::text=split_part(` + key + `,':s',2)))) `

	if kind == "series" || kind == "season" {
		filter := `media_id=(SELECT id FROM media_items WHERE media_type='series' AND tmdb_id=(` + tmdb + `)::bigint)`
		if kind == "season" {
			filter += ` AND season=(split_part(` + key + `,':s',2))::int`
		}
		// Fence the selective index lookup before membership joins. The shared bound
		// scope is consumed by per-episode lateral projections: materialize it once,
		// not once per returned/grouped episode (quadratic on cold planner stats).
		sql = strings.Replace(sql, "WITH ", `WITH parent_bindings AS MATERIALIZED(SELECT * FROM media_asset_bindings WHERE `+filter+`), `, 1)
		sql = strings.Replace(sql, "JOIN media_asset_bindings b ON", "JOIN parent_bindings b ON", 1)
		return strings.Replace(sql, "bound AS NOT MATERIALIZED", "bound AS MATERIALIZED", 1)
	}

	return strings.Replace(sql, `WHERE EXISTS(SELECT 1 FROM strm_entries`, `WHERE true `+condition+` AND EXISTS(SELECT 1 FROM strm_entries`, 1)
}
