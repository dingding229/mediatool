WITH facts AS MATERIALIZED (
 SELECT host(p.ip) AS ip,p.first_seen_at,p.last_seen_at,e.id AS session_id,NULL::uuid AS direct_id,e.user_id,e.device_id,e.started_at
 FROM media_playback_ips p JOIN media_playback_events e ON e.id=p.event_id
 WHERE ($1::text='' OR e.user_id=nullif($1,'')::uuid) AND ($2::text='' OR e.device_id=nullif($2,'')::uuid)
 UNION ALL
 SELECT client_ip,created_at,created_at,NULL::uuid,id,NULL::uuid,NULL::uuid,created_at
 FROM player_access_events
 WHERE $1::text='' AND $2::text='' AND allowed AND status=302 AND ip_public AND ip_source IN('direct','trusted_proxy')
), firsts AS (SELECT ip,min(first_seen_at) AS first_seen_at FROM facts GROUP BY ip)
