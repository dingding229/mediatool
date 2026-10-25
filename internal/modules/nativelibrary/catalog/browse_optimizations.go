package catalog

import (
	"fmt"
	"media-tool/internal/modules/nativelibrary"
	"strconv"
	"strings"
)

// Native protocol IDs are canonical decimal strings. Compare the indexed bigint
// column, never id::text; malformed/overflow/noncanonical IDs still match nothing.
func numericID(id string) int64 {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != id {
		return 0
	}
	return n
}
func numericIDs(ids []string) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if n := numericID(id); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

// Keep ALL eligible keys for parent user-data aggregation, but do not construct
// metadata headers for invisible types/IDs. Root pruning never replaces the
// outer library/parent/parental predicates or the final live permission check.
func pruneHeadersSQL(sql string, q nativelibrary.ItemQuery, args []any) (string, []any) {
	predicates := []string{}
	if len(q.IncludeItemTypes) > 0 {
		types := []string{}
		for _, kind := range q.IncludeItemTypes {
			types = append(types, itemKind(kind))
		}
		args = append(args, types)
		predicates = append(predicates, fmt.Sprintf("kind=ANY($%d::text[])", len(args)))
	}
	if len(q.IDs) > 0 {
		args = append(args, numericIDs(q.IDs))
		n := len(args)
		// Fence a bounded binding set before joins, including old/no-statistics
		// schemas. Without this a cold planner may rescan all bindings per ID.
		// Library IDs (including fallback libraries) need eligibility for all
		// their members: pruning them to zero media would hide the library.
		// The UNION branches are mutually exclusive, preserving asset counts.
		requested := fmt.Sprintf(`WITH requested_ids AS MATERIALIZED(SELECT entity_type,internal_key FROM native_item_ids WHERE id=ANY($%d::bigint[])),requested_media AS MATERIALIZED(
 SELECT m.id FROM media_items m JOIN requested_ids n ON m.media_type=CASE n.entity_type WHEN 'movie' THEN 'movie' ELSE 'series' END AND m.tmdb_id=substring(n.internal_key from '^tmdb:([0-9]+)')::bigint WHERE n.entity_type IN('movie','series','season','episode')
 ),requested_bindings AS MATERIALIZED(
 SELECT *FROM media_asset_bindings WHERE media_id IN(SELECT id FROM requested_media) AND NOT EXISTS(SELECT 1 FROM requested_ids WHERE entity_type='library')
 UNION ALL SELECT *FROM media_asset_bindings WHERE EXISTS(SELECT 1 FROM requested_ids WHERE entity_type='library')
 ), `, n)
		sql = strings.Replace(sql, "WITH ", requested, 1)
		sql = strings.Replace(sql, "JOIN media_asset_bindings b ON", "JOIN requested_bindings b ON", 1)
		sql = strings.Replace(sql, "bound AS NOT MATERIALIZED", "bound AS MATERIALIZED", 1)
		predicates = append(predicates, fmt.Sprintf("(kind,internal_key) IN(SELECT entity_type,internal_key FROM native_item_ids WHERE id=ANY($%d::bigint[]))", n))
		roots := " AND (EXISTS(SELECT 1 FROM requested_ids WHERE entity_type='library') OR (e.media_type,e.tmdb_id) IN(SELECT CASE entity_type WHEN 'movie' THEN 'movie' ELSE 'series' END,substring(internal_key from '^tmdb:([0-9]+)')::bigint FROM requested_ids WHERE entity_type IN('movie','series','season','episode'))) "
		sql = strings.Replace(sql, "WHERE NOT l.hidden", "WHERE NOT l.hidden"+roots, 1)
		// Movie specialization already inserted a type predicate before NOT hidden.
		sql = strings.Replace(sql, "WHERE e.media_type='movie' AND NOT l.hidden", "WHERE e.media_type='movie' AND NOT l.hidden"+roots, 1)
	}
	if len(predicates) > 0 {
		sql = strings.Replace(sql, "FROM keyed g JOIN media_items b", "FROM (SELECT * FROM keyed WHERE "+strings.Join(predicates, " AND ")+") g JOIN media_items b", 1)
	}
	return sql, args
}

// A cold planner may estimate keyed/page_keys at one row and inline a join that
// rebuilds all metadata for every selected key. Fence the small key intersection
// BEFORE provider/user-data projection. Reuse the same permission-filtered keys
// and parent aggregates; never re-run the physical eligibility scan.
func selectedPageSQL() string {
	start := strings.Index(browseCTE, ",headers AS")
	end := strings.Index(browseCTE, personalCTE)
	headers := strings.Replace(browseCTE[start:end], headerMetadata, fullPageMetadata+`||`+parentTextMetadata, 1)
	headers = strings.Replace(headers, "FROM keyed g JOIN media_items b", "FROM page_grouped g JOIN media_items b", 1)
	headers = strings.ReplaceAll(headers, "headers", "page_headers")
	headers = strings.ReplaceAll(headers, "raw_item_rows", "page_raw_item_rows")
	headers = strings.ReplaceAll(headers, "media_rows", "page_media_rows")
	personal := personalCTE[strings.Index(personalCTE, ",item_rows AS"):]
	personal = strings.ReplaceAll(personal, "raw_item_rows", "page_raw_item_rows")
	personal = strings.Replace(personal, ",item_rows AS NOT MATERIALIZED", ",page_item_rows AS MATERIALIZED", 1)
	return `,page_grouped AS MATERIALIZED(SELECT * FROM keyed WHERE (kind,internal_key) IN(SELECT kind,internal_key FROM page_keys))` + headers + personal
}

func projectionOrder(order []string) ([]string, []string, error) {
	cols := []string{"kind", "internal_key", "id"}
	out := make([]string, 0, len(order)+1)
	for i, entry := range order {
		at := strings.LastIndex(entry, " ASC")
		if d := strings.LastIndex(entry, " DESC"); d > at {
			at = d
		}
		if at < 0 {
			return nil, nil, nativelibrary.ErrInvalidQuery
		}
		alias := fmt.Sprintf("catalog_sort_%d", i)
		cols = append(cols, entry[:at]+" AS "+alias)
		out = append(out, alias+entry[at:])
	}
	return cols, append(out, "id::bigint ASC NULLS LAST,kind,internal_key"), nil
}
