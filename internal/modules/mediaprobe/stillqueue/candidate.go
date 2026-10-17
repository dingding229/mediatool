package stillqueue

import "media-tool/internal/modules/nativelibrary/catalog"

// Canonical source: newest exposable, probeable asset across all non-hidden TV
// libraries, then asset UUID ascending (the unified browse source order).
// The caller supplies an EPISODE item ID, never a client-provided file path.
const candidateSQL = `WITH visible AS (` + catalog.ExposableAssetsSQL + `), target AS (
 SELECT n.*,regexp_match(n.internal_key,'^tmdb:([0-9]{1,19}):s([0-9]{1,9})e([0-9]{1,9})$') parts
 FROM native_item_ids n WHERE n.id=$1 AND n.entity_type='episode') SELECT n.id,
 coalesce(s.asset_id::text,'') AS asset_id,coalesce(s.fingerprint,'') AS fingerprint,coalesce(s.ticks,0)::bigint AS ticks,coalesce(s.has_still,false) AS has_still
 FROM target n LEFT JOIN LATERAL (
 SELECT e.asset_id,p.result_fingerprint fingerprint,
 CASE WHEN (p.result->>'durationTicks')~'^[0-9]{1,18}$' THEN (p.result->>'durationTicks')::bigint ELSE 0 END ticks,
 EXISTS(SELECT 1 FROM native_tmdb_details d WHERE d.tmdb_id=e.tmdb_id AND d.season_number=e.season AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 AND ((d.tmdb_type='episode' AND d.episode_number=epn.first_episode AND coalesce(d.payload->>'stillPath','')<>'')
 OR (d.tmdb_type='season' AND EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(d.payload->'episodes')='array' THEN d.payload->'episodes' ELSE '[]'::jsonb END) ep
 WHERE ep->>'number'=epn.first_episode::text AND coalesce(ep->>'stillPath','')<>'')))) has_still
 FROM visible e JOIN cloud_files f ON f.id=e.cloud_file_id JOIN media_probe_results p ON p.cloud_file_id=e.cloud_file_id
 CROSS JOIN LATERAL (SELECT coalesce(min(value::int),e.episode) first_episode
 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(e.episodes)='array' THEN e.episodes ELSE '[]'::jsonb END)
 WHERE value~'^[0-9]{1,9}$') epn
 WHERE n.retired_at IS NULL AND e.media_type='series' AND e.season>=0
 AND e.tmdb_id=CASE WHEN n.parts[1]::numeric BETWEEN 1 AND 9223372036854775807 THEN n.parts[1]::bigint END
 AND e.season=n.parts[2]::int AND n.parts[3]::int>0
 AND epn.first_episode=n.parts[3]::int
 AND n.internal_key='tmdb:'||e.tmdb_id||':s'||e.season||'e'||epn.first_episode
 AND EXISTS(SELECT 1 FROM native_asset_libraries al JOIN native_libraries lib ON lib.id=al.library_id WHERE al.asset_id=e.asset_id AND NOT lib.hidden AND lib.collection_type='tvshows')
 AND jsonb_typeof(p.result->'durationTicks')='number' AND p.probed_at IS NOT NULL AND p.result_fingerprint~'^[0-9a-f]{64}$' AND p.file_fingerprint=p.result_fingerprint
 AND p.result_fingerprint=native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata)
 AND CASE WHEN (p.result->>'durationTicks')~'^[0-9]{1,18}$' THEN (p.result->>'durationTicks')::bigint ELSE 0 END>0
 ORDER BY e.created_at DESC,e.asset_id LIMIT 1
 ) s ON true WHERE n.id=$1 AND n.entity_type='episode'`
