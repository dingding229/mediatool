package migrations_test

import (
	"encoding/json"
	"media-tool/internal/modules/nativelibrary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestT3EffectiveMetadataLargeLibraryPlan(t *testing.T) {
	if os.Getenv("T3_PERF") != "1" {
		t.Skip("explicit 100k synthetic performance workload")
	}
	f, tr := b5Fixture(t)
	b5SeedMovies(t, f, 100000)
	if _, e := f.pool.Exec(f.ctx, `ANALYZE media_items;ANALYZE media_asset_bindings;ANALYZE media_assets;ANALYZE cloud_files;ANALYZE strm_entries;ANALYZE native_asset_libraries;ANALYZE native_item_ids`); e != nil {
		t.Fatal(e)
	}
	dir := os.Getenv("T3_PERF_DIR")
	if dir == "" {
		t.Fatal("private evidence dir required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	results := []map[string]any{}
	run := func(name string, q nativelibrary.ItemQuery) {
		t.Log("workload", name)
		tr.mu.Lock()
		tr.queries = nil
		tr.mu.Unlock()
		start := time.Now()
		items, total := f.query(t, q)
		elapsed := time.Since(start)
		if len(items) > q.Limit || elapsed > 5*time.Second {
			t.Fatal("unbounded/slow effective query", name, elapsed)
		}
		tr.mu.Lock()
		queries := append([]b5Query{}, tr.queries...)
		tr.mu.Unlock()
		for _, x := range queries {
			if !strings.Contains(x.SQL, "filtered AS") {
				continue
			}
			var plan json.RawMessage
			if e := f.pool.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+x.SQL, x.Args...).Scan(&plan); e != nil {
				t.Fatal(e)
			}
			record := map[string]any{"name": name, "rows": 100000, "total": total, "returned": len(items), "requestMs": float64(elapsed) / 1e6, "sqlMs": float64(x.Duration) / 1e6, "plan": plan}
			results = append(results, record)
			if name == "empty-overlays" && os.Getenv("T3_BASELINE_CTE") != "" {
				base, e := os.ReadFile(os.Getenv("T3_BASELINE_CTE"))
				if e != nil {
					t.Fatal(e)
				}
				a := strings.Index(x.SQL, ",episode_metadata AS")
				b := strings.Index(x.SQL, ",visible_episode_data AS")
				if a < 0 || b <= a {
					t.Fatal("baseline boundaries")
				}
				fragment := string(base)
				prune := regexp.MustCompile(`FROM \(SELECT \* FROM keyed WHERE kind=ANY\(\$[0-9]+::text\[\]\)\) g JOIN media_items b`).FindString(x.SQL)
				if prune != "" {
					fragment = strings.Replace(fragment, "FROM keyed g JOIN media_items b", prune, 1)
				}
				baseline := x.SQL[:a] + fragment + x.SQL[b:]
				var oldPlan json.RawMessage
				if e = f.pool.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+baseline, x.Args...).Scan(&oldPlan); e != nil {
					t.Fatal(e)
				}
				results = append(results, map[string]any{"name": "v0.33.1-baseline", "plan": oldPlan})
			}
		}
		partial, _ := json.MarshalIndent(results, "", " ")
		if e := os.WriteFile(filepath.Join(dir, "100k-plans.json"), partial, 0600); e != nil {
			t.Fatal(e)
		}
	}
	q := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie", "Movie"}, SortBy: []string{"SortName"}, Limit: 30}
	run("empty-overlays", q)
	f.exec(t, `INSERT INTO media_metadata_overrides(media_id,entity_type,overrides,revision) SELECT id,'movie',jsonb_build_object('title','Manual '||tmdb_id,'sortName','Manual '||tmdb_id,'year',2026,'genres',jsonb_build_array('Manual')),1 FROM media_items WHERE tmdb_id%10=0`)
	if _, e := f.pool.Exec(f.ctx, `ANALYZE media_metadata_overrides`); e != nil {
		t.Fatal(e)
	}
	run("10pct-overlays", q)
	q.SearchTerm = "Manual 990010"
	run("effective-title-search", q)
	q.SearchTerm = ""
	q.Years = []string{"2026"}
	q.Genres = []string{"manual"}
	run("effective-filter", q)
	raw, e := json.MarshalIndent(results, "", " ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "100k-plans.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
