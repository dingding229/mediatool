package catalog

import (
	"encoding/json"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
)

func classificationInput(i metadata.Item, snapshot []byte) metadata.Item {
	var envelope struct {
		Facts *classification.Facts `json:"classification"`
	}
	if json.Unmarshal(snapshot, &envelope) == nil && envelope.Facts != nil {
		return envelope.Facts.Apply(i)
	}
	return i
}

// One source selection for detail AND list/filter. Failed attempts may retain a
// valid successful snapshot; never fetch on a browse request or borrow a locale.
const managedSnapshots = managedMembers + ` LEFT JOIN LATERAL(
 SELECT payload FROM native_tmdb_details d WHERE d.tmdb_type=m.media_type AND d.tmdb_id=m.tmdb_id AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') ORDER BY d.fetched_at DESC LIMIT 1)d ON true `

// Mirror Facts.Valid using guarded JSON access: malformed private cache data
// degrades to ingest facts, never turns a listing into a cast error or divergent
// classification between the SQL list and Go detail projector.
const validFactsSQL = `(d.payload#>>'{classification,version}'='1' AND d.payload#>'{classification,version}'='1'::jsonb
 AND CASE WHEN jsonb_typeof(d.payload#>'{classification,genreIds}')='array' THEN jsonb_array_length(d.payload#>'{classification,genreIds}')<=200 ELSE false END
 AND CASE WHEN jsonb_typeof(d.payload#>'{classification,countries}')='array' THEN jsonb_array_length(d.payload#>'{classification,countries}')<=128 ELSE false END
 AND coalesce(d.payload#>>'{classification,originalLanguage}','')~'^([a-z]{2,3})?$'
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(d.payload#>'{classification,genreIds}')='array' THEN d.payload#>'{classification,genreIds}' ELSE '[]'::jsonb END) v
 WHERE jsonb_typeof(v)<>'number' OR CASE WHEN v::text~'^[1-9][0-9]{0,5}$' THEN v::text::int>100000 ELSE true END)
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(d.payload#>'{classification,countries}')='array' THEN d.payload#>'{classification,countries}' ELSE '[]'::jsonb END) v WHERE jsonb_typeof(v)<>'string' OR (v#>>'{}')!~'^[A-Z]{2}$'))`
const ingestFactsSQL = `CASE WHEN jsonb_typeof(m.metadata)='object' THEN m.metadata ELSE '{}'::jsonb END`
const mergedFactsSQL = `CASE WHEN ` + validFactsSQL + ` THEN (` + ingestFactsSQL + `)||jsonb_strip_nulls(jsonb_build_object(
 'genreIds',CASE WHEN CASE WHEN jsonb_typeof(m.metadata->'genreIds')='array' THEN jsonb_array_length(m.metadata->'genreIds')>0 ELSE false END THEN m.metadata->'genreIds' ELSE d.payload#>'{classification,genreIds}' END,
 'countries',CASE WHEN CASE WHEN jsonb_typeof(m.metadata->'countries')='array' THEN jsonb_array_length(m.metadata->'countries')>0 ELSE false END THEN m.metadata->'countries' ELSE d.payload#>'{classification,countries}' END,
 'originalLanguage',coalesce(nullif(m.metadata->>'originalLanguage',''),d.payload#>>'{classification,originalLanguage}'))) ELSE ` + ingestFactsSQL + ` END`
const managedClassificationMembers = managedSnapshots + ` LEFT JOIN LATERAL(SELECT ` + mergedFactsSQL + ` metadata)c ON true `
