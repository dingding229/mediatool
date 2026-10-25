package catalog

import "media-tool/internal/modules/metadataedit"

// The classification lateral remains based exclusively on ingest/provider facts.
// Display overlays are joined separately and participate in WHERE before paging.
func managedEffectiveMembers(kind string) string {
	ep := metadataedit.LeafSQL("sd.payload", "a.media_identity", "b.season", "coalesce((SELECT min(value::int)FROM jsonb_array_elements_text(b.episodes)),b.episode)")
	return managedClassificationMembers + ` LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=m.tmdb_id AND sd.season_number=b.season AND sd.fetched_at IS NOT NULL AND sd.language=coalesce((SELECT nullif(settings->>'language','')FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') ` +
		metadataedit.Join("o", "m.id", kind, "CASE WHEN "+kind+" IN('season','episode') THEN b.season ELSE -1 END", "CASE WHEN "+kind+"='episode' THEN coalesce((SELECT min(value::int)FROM jsonb_array_elements_text(b.episodes)),b.episode) ELSE -1 END") +
		` LEFT JOIN LATERAL(SELECT ` + metadataedit.BaseSQL("coalesce(m.title,f.metadata->>'name',regexp_replace(f.full_path,'^.*/','','g'))", "m.release_year", "m.metadata", kind, "b.season", "coalesce(b.episode,0)", "d.payload", "sd.payload", ep) + `||coalesce(o.overrides,'{}'::jsonb) metadata)v ON true `
}
