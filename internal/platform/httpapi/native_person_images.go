package httpapi

import (
	"context"
	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"strconv"
)

// A person has no global/public image entitlement. At least one live, visible,
// parental-allowed title containing this person must evidence its profile path.
func (r *NativeImageResolver) lookupPerson(ctx context.Context, id int64, req embyapi.ImageRequest) (imageMeta, error) {
	libraries, codes, unrated, err := catalog.ImageScope(ctx, *req.User, req.Permissions)
	if err != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	rows, err := r.DB.Query(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`)
 SELECT DISTINCT coalesce(p->>'imagePath',''),al.library_id,coalesce(nullif(rd.payload->>'officialRating',''),m.metadata->>'officialRating','')
 FROM eligible e JOIN media_items m ON m.id=e.media_id
 JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 JOIN native_tmdb_details d ON d.tmdb_id=e.tmdb_id AND (d.tmdb_type=e.media_type OR e.media_type='series' AND d.tmdb_type='season' AND d.season_number=e.season)
 LEFT JOIN native_tmdb_details rd ON rd.tmdb_id=e.tmdb_id AND rd.tmdb_type=e.media_type AND rd.fetched_at IS NOT NULL AND rd.language=d.language
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(d.payload->'people')='array' THEN d.payload->'people' ELSE '[]'::jsonb END) p
 WHERE NOT l.hidden AND e.media_type=CASE l.collection_type WHEN 'movies' THEN 'movie' ELSE 'series' END AND l.id=ANY($1::text[]) AND p->>'id'=$2::text AND p->>'imagePath'~$5::text AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')
 AND ($3::boolean OR btrim(regexp_replace(upper(btrim(coalesce(nullif(rd.payload->>'officialRating',''),m.metadata->>'officialRating',''))),'^(CN:|US:)\s*',''))=ANY($4::text[]))
 ORDER BY 1,2,3 LIMIT 200`, libraries, strconv.FormatInt(id, 10), unrated, codes, nativelibrary.TMDBImageSourcePattern)
	if err != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	defer rows.Close()
	fallback := ""
	for rows.Next() {
		var path, library, rating string
		if err := rows.Scan(&path, &library, &rating); err != nil {
			return imageMeta{}, embyapi.ErrImageUnavailable
		}
		err := req.Permissions.ItemAllowed(ctx, *req.User, contracts.ItemAccess{LibraryID: library, OfficialRating: rating})
		if err == contracts.ErrForbidden || err == contracts.ErrParentalDenied {
			continue
		}
		if err != nil {
			return imageMeta{}, embyapi.ErrImageUnavailable
		}
		path = nativelibrary.ImagePath(path)
		if path == "" {
			continue
		}
		if nativelibrary.ImageTag(path) == req.Tag {
			return imageMeta{poster: path}, nil
		}
		if fallback == "" {
			fallback = path
		}
	}
	if rows.Err() != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	if fallback == "" {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	return imageMeta{poster: fallback}, nil
}
