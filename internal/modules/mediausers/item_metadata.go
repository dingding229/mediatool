package mediausers

// ItemMetadataJoins supplies structured binding season/episode metadata to the
// administrator history/session projections. Alias i is a native_item_ids row.
// It never parses season/episode from the legacy internal key.
const ItemMetadataJoins = ` LEFT JOIN LATERAL(SELECT id,title,tmdb_id FROM media_items WHERE tmdb_id=substring(i.internal_key from '^tmdb:([0-9]+)')::bigint AND media_type=CASE WHEN i.entity_type='movie' THEN 'movie' ELSE 'series' END ORDER BY id LIMIT 1)m ON true
 LEFT JOIN LATERAL(SELECT b.season,b.episode,ep.value->>'title' episode_title FROM media_asset_bindings b JOIN media_assets a ON a.id=b.asset_id
 LEFT JOIN LATERAL(SELECT value FROM jsonb_array_elements(CASE WHEN jsonb_typeof(a.media_identity->'episodeDetails')='array' THEN a.media_identity->'episodeDetails' ELSE '[]'::jsonb END) WHERE value->>'season'=b.season::text AND value->>'number'=b.episode::text LIMIT 1)ep ON true
 WHERE b.media_id=m.id AND i.entity_type='episode' AND i.internal_key='tmdb:'||m.tmdb_id||':s'||b.season||'e'||b.episode ORDER BY b.asset_id LIMIT 1)b ON true`
