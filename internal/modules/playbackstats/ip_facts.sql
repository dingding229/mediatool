SELECT p.ip,count(DISTINCT p.user_id),count(DISTINCT p.device_id),
 count(DISTINCT p.session_id)+count(DISTINCT p.direct_id),min(f.first_seen_at),max(p.last_seen_at),
 coalesce(jsonb_agg(DISTINCT jsonb_build_object('id',u.id::text,'name',coalesce(nullif(u.display_name,''),u.username))) FILTER(WHERE u.id IS NOT NULL),'[]'::jsonb),
 coalesce(jsonb_agg(DISTINCT jsonb_build_object('id',d.id::text,'name',d.name,'client',d.client)) FILTER(WHERE d.id IS NOT NULL),'[]'::jsonb),
 count(DISTINCT p.session_id),count(DISTINCT p.direct_id)
 FROM facts p JOIN firsts f ON f.ip=p.ip LEFT JOIN media_users u ON u.id=p.user_id LEFT JOIN media_user_devices d ON d.user_id=p.user_id AND d.id=p.device_id
 WHERE p.started_at >= $3 AND p.started_at <= $4
 GROUP BY p.ip ORDER BY max(p.last_seen_at) DESC,p.ip LIMIT 100001
