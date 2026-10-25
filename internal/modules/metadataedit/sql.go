package metadataedit

import "strings"

// BaseSQL is pure SQL over joined rows, not a per-row DB lookup. Every caller
// supplies trusted SQL expressions. Manual keys are appended AFTER this base.
func BaseSQL(title, year, meta, kind, season, episode, root, sd, ep string) string {
	template := `(coalesce(META,'{}'::jsonb)||jsonb_strip_nulls(jsonb_build_object(
 'originalTitle',nullif(ROOT->>'originalTitle',''),'sortName',nullif(ROOT->>'sortName',''),
 'overview',nullif(ROOT->>'overview',''),'genres',nullif(ROOT->'genres','[]'::jsonb),
 'premiereDate',nullif(ROOT->>'premiereDate',''),'communityRating',ROOT->'communityRating',
 'officialRating',nullif(ROOT->>'officialRating',''),'tagline',ROOT->'tagline','tags',ROOT->'tags'))
 ||CASE WHEN KIND IN('season','episode') THEN jsonb_build_object(
 'originalTitle',coalesce(SD->>'originalTitle',''),'sortName',coalesce(SD->>'sortName',''),
 'overview',coalesce(SD->>'overview',''),'premiereDate',SD->'premiereDate',
 'communityRating',SD->'communityRating','tagline',SD->'tagline','tags',SD->'tags') ELSE '{}'::jsonb END
 ||CASE WHEN KIND='episode' THEN jsonb_build_object('overview',coalesce(EP->>'overview',''),'premiereDate',nullif(EP->>'airDate',''),'communityRating',EP->'communityRating') ELSE '{}'::jsonb END
 ||jsonb_build_object('title',CASE KIND WHEN 'season' THEN coalesce(nullif(SD->>'name',''),CASE WHEN (SEASON)=0 THEN '特别篇' ELSE '第 '||(SEASON)||' 季' END)
 WHEN 'episode' THEN coalesce(nullif(EP->>'name',''),nullif(EP->>'title',''),'第 '||(EPISODE)||' 集') ELSE TITLE END,
 'year',CASE WHEN KIND='season' AND coalesce(SD->>'premiereDate','')~'^[0-9]{4}-' THEN to_jsonb(left(SD->>'premiereDate',4)::int)
 WHEN KIND='episode' AND coalesce(EP->>'airDate','')~'^[0-9]{4}-' THEN to_jsonb(left(EP->>'airDate',4)::int)
 WHEN YEAR~'^[0-9]{4}$' THEN to_jsonb(YEAR::int) ELSE 'null'::jsonb END))`
	return strings.NewReplacer("TITLE", title, "YEAR", year, "META", meta, "KIND", kind, "SEASON", season, "EPISODE", episode, "ROOT", root, "SD", sd, "EP", ep).Replace(template)
}
func Join(alias, media, kind, season, episode string) string {
	return " LEFT JOIN media_metadata_overrides " + alias + " ON " + alias + ".media_id=" + media + " AND " + alias + ".entity_type=" + kind + " AND " + alias + ".season_number=" + season + " AND " + alias + ".episode_number=" + episode + " "
}

// HeaderSQL builds only searchable/sortable/filterable text. The full base (in
// particular Overview) is projected again for page keys only. This avoids copying
// large provider descriptions and classification JSON for every library member.
func HeaderSQL(title, year, meta, kind, season, episode, root, sd, ep, overlay string) string {
	template := `(coalesce(META-'title'-'year'-'overview','{}'::jsonb)||jsonb_strip_nulls(jsonb_build_object(
 'originalTitle',nullif(ROOT->>'originalTitle',''),'sortName',nullif(ROOT->>'sortName',''),
 'genres',nullif(ROOT->'genres','[]'::jsonb),'officialRating',nullif(ROOT->>'officialRating',''),
 'premiereDate',nullif(ROOT->>'premiereDate',''),'communityRating',ROOT->'communityRating'))
 ||CASE WHEN KIND IN('season','episode') THEN jsonb_build_object('originalTitle',coalesce(SD->>'originalTitle',''),'sortName',coalesce(SD->>'sortName',''),'premiereDate',SD->'premiereDate','communityRating',SD->'communityRating') ELSE '{}'::jsonb END
 ||CASE WHEN KIND='episode' THEN jsonb_build_object('premiereDate',nullif(EP->>'airDate',''),'communityRating',EP->'communityRating') ELSE '{}'::jsonb END
 ||coalesce(OV-'overview'-'tagline'-'tags','{}'::jsonb))`
	return strings.NewReplacer("TITLE", title, "YEAR", year, "META", meta, "KIND", kind, "SEASON", season, "EPISODE", episode, "ROOT", root, "SD", sd, "EP", ep, "OV", overlay).Replace(template)
}

// A statement-scoped InitPlan skips 100k empty PK probes on a library that has
// never been edited. Once overlays exist, the composite PK still bounds each
// logical identity lookup. Presence and values share the statement's snapshot.
func HeaderJoin(alias, media, kind, season, episode string) string {
	return Join(alias, media, kind, season, episode) + ` AND (SELECT EXISTS(SELECT 1 FROM media_metadata_overrides)) `
}
