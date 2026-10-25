package imageedit

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
	"strings"
)

// NativePage is one bounded indexed join for current page identities and parents.
func NativePage(ctx context.Context, db *pgxpool.Pool, ids []string) (map[string]map[string]Image, error) {
	out := map[string]map[string]Image{}
	if len(ids) == 0 {
		return out, nil
	}
	numbers := []int64{}
	for _, id := range ids {
		n, e := strconv.ParseInt(id, 10, 64)
		if e == nil {
			numbers = append(numbers, n)
		}
	}
	rows, e := db.Query(ctx, `SELECT n.id::text,k.key,f.id::text,f.digest FROM native_item_ids n
 JOIN media_items m ON m.media_type=CASE n.entity_type WHEN 'movie' THEN 'movie' ELSE 'series' END AND m.tmdb_id=substring(n.internal_key from '^tmdb:([0-9]+)')::bigint
 JOIN media_image_overrides o ON o.media_id=m.id AND o.entity_type=n.entity_type AND o.season_number=coalesce(substring(n.internal_key from ':s([0-9]+)')::int,-1) AND o.episode_number=coalesce(substring(n.internal_key from 'e([0-9]+)$')::int,-1)
 CROSS JOIN LATERAL jsonb_each_text(o.images)k JOIN media_image_files f ON f.id=k.value::uuid
 WHERE n.id=ANY($1::bigint[]) AND n.retired_at IS NULL`, numbers)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, k string
		var im Image
		if e = rows.Scan(&id, &k, &im.ID, &im.Tag); e != nil {
			return out, e
		}
		if out[id] == nil {
			out[id] = map[string]Image{}
		}
		out[id][k] = im
	}
	return out, rows.Err()
}

// ManagedPrimary resolves only the already paginated IDs, never the whole library.
func ManagedPrimary(ctx context.Context, db *pgxpool.Pool, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	type wanted struct {
		ID    string `json:"id"`
		Media string `json:"media"`
		Kind  string `json:"kind"`
		S     int    `json:"s"`
		E     int    `json:"e"`
	}
	list := []wanted{}
	for _, id := range ids {
		w := wanted{ID: id, S: -1, E: -1}
		p := strings.Split(id, "_")
		if len(p) >= 2 {
			w.Kind, w.Media = p[0], p[1]
			if len(p) > 2 {
				w.S, _ = strconv.Atoi(p[2])
			}
			if len(p) > 3 {
				w.E, _ = strconv.Atoi(p[3])
			}
		}
		list = append(list, w)
	}
	raw, e := json.Marshal(list)
	if e != nil {
		return out, e
	}
	rows, e := db.Query(ctx, `WITH wanted AS(SELECT * FROM jsonb_to_recordset($1)w(id text,media text,kind text,s int,e int)), targets AS(
 SELECT w.id,coalesce(nullif(w.media,'')::uuid,b.media_id) media_id,coalesce(nullif(w.kind,''),CASE m.media_type WHEN 'series' THEN 'episode' ELSE 'movie' END) kind,
 CASE WHEN w.kind<>'' THEN w.s WHEN m.media_type='series' THEN coalesce(b.season,-1) ELSE -1 END s,
 CASE WHEN w.kind<>'' THEN w.e WHEN m.media_type='series' THEN coalesce((SELECT min(value::int)FROM jsonb_array_elements_text(b.episodes)),b.episode,-1) ELSE -1 END e
 FROM wanted w LEFT JOIN media_asset_bindings b ON w.media='' AND b.asset_id=CASE WHEN w.media='' THEN w.id::uuid END AND b.state='ready' LEFT JOIN media_items m ON m.id=b.media_id)
 SELECT t.id,f.digest FROM targets t JOIN media_image_overrides o ON o.media_id=t.media_id AND o.entity_type=t.kind AND o.season_number=t.s AND o.episode_number=t.e JOIN media_image_files f ON f.id=(o.images->>'Primary')::uuid`, raw)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if e = rows.Scan(&id, &tag); e != nil {
			return out, e
		}
		out[id] = URL(id, "Primary", tag)
	}
	return out, rows.Err()
}
