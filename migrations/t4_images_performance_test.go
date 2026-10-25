package migrations_test

import (
	"encoding/json"
	"media-tool/internal/modules/nativelibrary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestT4ImagePageLargeLibraryPlan(t *testing.T) {
	if os.Getenv("T4_PERF") != "1" {
		t.Skip("explicit bounded 100k workload")
	}
	f, tr := b5Fixture(t)
	b5SeedMovies(t, f, 100000)
	// 100k image registrations and locks exercise index lookup, not an empty table shortcut.
	f.exec(t, `INSERT INTO media_image_files(id,digest,filename,content_type,width,height,size_bytes,source) SELECT id,repeat('a',64),id||'.png','image/png',1,1,1,'upload' FROM media_items`)
	f.exec(t, `INSERT INTO media_image_overrides(media_id,entity_type,images) SELECT id,'movie',jsonb_build_object('Primary',id::text) FROM media_items`)
	if _, e := f.pool.Exec(f.ctx, `ANALYZE media_items;ANALYZE media_image_files;ANALYZE media_image_overrides;ANALYZE media_asset_bindings;ANALYZE media_assets;ANALYZE cloud_files;ANALYZE strm_entries;ANALYZE native_asset_libraries;ANALYZE native_item_ids`); e != nil {
		t.Fatal(e)
	}
	tr.mu.Lock()
	tr.queries = nil
	tr.mu.Unlock()
	start := time.Now()
	items, total := f.query(t, nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 24})
	elapsed := time.Since(start)
	if total != 100000 || len(items) != 24 || elapsed > 5*time.Second {
		t.Fatal(total, len(items), elapsed)
	}
	tr.mu.Lock()
	queries := append([]b5Query{}, tr.queries...)
	tr.mu.Unlock()
	evidence := []map[string]any{}
	for _, q := range queries {
		if !strings.Contains(q.SQL, "media_image_overrides") {
			continue
		}
		var plan json.RawMessage
		if e := f.pool.QueryRow(f.ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+q.SQL, q.Args...).Scan(&plan); e != nil {
			t.Fatal(e)
		}
		evidence = append(evidence, map[string]any{"sqlMs": float64(q.Duration) / 1e6, "plan": plan})
	}
	if len(evidence) != 1 {
		t.Fatal("image page should be one batch query", len(evidence))
	}
	raw, e := json.MarshalIndent(map[string]any{"librarySize": 100000, "imageFiles": 100000, "pageSize": 24, "requestMs": float64(elapsed) / 1e6, "imageQueries": evidence}, "", " ")
	if e != nil {
		t.Fatal(e)
	}
	dir := os.Getenv("T4_PERF_DIR")
	if dir == "" {
		t.Fatal("evidence dir required")
	}
	if e = os.WriteFile(filepath.Join(dir, "100k-plans.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
