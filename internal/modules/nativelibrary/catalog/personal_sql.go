package catalog

// $1 is visible libraries, $2 is the selected library, $3 is the authenticated user.
// Scope applies before parent aggregation and pagination.
var personalCTE = `,visible_episode_data AS (
 SELECT g.tmdb_id,g.s,coalesce(d.played,false) played,coalesce(d.play_count,0) play_count,d.last_played_at
 FROM keyed g
 LEFT JOIN LATERAL(SELECT id FROM native_item_ids WHERE entity_type='episode' AND internal_key=g.internal_key LIMIT 1)n ON true
 LEFT JOIN media_user_item_data d ON d.item_id=n.id AND d.user_id=$3::uuid
 WHERE g.kind='episode'
),episode_user_totals AS MATERIALIZED (
 SELECT tmdb_id,scope_s,count(*) total,count(*) FILTER(WHERE NOT played) unplayed,sum(play_count) play_count,max(last_played_at) last_played_at
 FROM visible_episode_data CROSS JOIN LATERAL(VALUES(-1),(s)) scopes(scope_s) GROUP BY tmdb_id,scope_s
),item_rows AS NOT MATERIALIZED (
 SELECT m.*,CASE WHEN m.kind IN('series','season') THEN coalesce(t.total>0 AND t.unplayed=0,false) ELSE coalesce(d.played,false) END user_played,
 CASE WHEN m.kind IN('series','season') THEN 0 ELSE coalesce(d.position_ticks,0) END user_position,
 CASE WHEN m.kind IN('series','season') THEN coalesce(t.play_count,0) ELSE coalesce(d.play_count,0) END user_count,
 coalesce(d.favorite,false) user_favorite,
 CASE WHEN m.kind IN('series','season') THEN greatest(t.last_played_at,d.last_played_at) ELSE d.last_played_at END user_last_played,
 CASE WHEN m.kind IN('series','season') THEN coalesce(t.unplayed,0) END user_unplayed,
 CASE WHEN m.kind IN('series','season') THEN coalesce(100.0*(t.total-t.unplayed)/nullif(t.total,0),0) END user_percentage
 FROM raw_item_rows m LEFT JOIN media_user_item_data d ON d.item_id=m.id::bigint AND d.user_id=$3::uuid
 LEFT JOIN episode_user_totals t ON m.kind IN('series','season') AND t.tmdb_id=m.tmdb_id AND t.scope_s=m.s
) `

// Window ranking operates only on permission-filtered episodes. Both the
// completion frontier and next candidate ignore hidden/missing assets.
func nextUpCTE(where string) string {
	return `,personal_visible AS MATERIALIZED(SELECT kind,internal_key,id,tmdb_id,s,e,user_played,user_last_played FROM item_rows WHERE ` + where + `),recent_shows AS (
 SELECT tmdb_id,max(user_last_played) last_watch FROM personal_visible GROUP BY tmdb_id
 HAVING max(user_last_played)>=statement_timestamp()-interval '90 days' ORDER BY last_watch DESC,tmdb_id LIMIT 501
),completed_frontier AS (
 SELECT DISTINCT ON(v.tmdb_id) v.tmdb_id,v.s,v.e FROM personal_visible v JOIN recent_shows r USING(tmdb_id)
 WHERE v.user_played ORDER BY v.tmdb_id,v.s DESC,v.e DESC
),ranked_next AS (
 SELECT v.*,r.last_watch,row_number() OVER(PARTITION BY v.tmdb_id ORDER BY v.s,v.e,v.id::bigint) next_rank
 FROM personal_visible v JOIN recent_shows r USING(tmdb_id) LEFT JOIN completed_frontier c USING(tmdb_id)
 WHERE NOT v.user_played AND (c.tmdb_id IS NULL OR (v.s,v.e)>(c.s,c.e))
),filtered AS(SELECT * FROM ranked_next WHERE next_rank=1)`
}
