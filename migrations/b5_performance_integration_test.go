package migrations_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type b5Query struct {
	SQL      string
	Args     []any
	Duration time.Duration
}
type b5Tracer struct {
	base    *browseTrace
	mu      sync.Mutex
	queries []b5Query
	explain bool
}
type b5TraceKey struct{}

func (tr *b5Tracer) TraceQueryStart(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	ctx = tr.base.TraceQueryStart(ctx, c, d)
	return context.WithValue(ctx, b5TraceKey{}, b5Query{SQL: d.SQL, Args: append([]any(nil), d.Args...), Duration: time.Duration(time.Now().UnixNano())})
}
func (tr *b5Tracer) TraceQueryEnd(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryEndData) {
	tr.base.TraceQueryEnd(ctx, c, d)
	q, _ := ctx.Value(b5TraceKey{}).(b5Query)
	q.Duration = time.Duration(time.Now().UnixNano()) - q.Duration
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if !tr.explain {
		tr.queries = append(tr.queries, q)
	}
}
func b5SeedMovies(t *testing.T, f browseFixture, count int) {
	t.Helper()
	f.exec(t, `CREATE TEMP TABLE native_browse_seed AS SELECT i,md5('browse-movie-'||i)::uuid media,md5('browse-file-'||i)::uuid file,md5('browse-asset-'||i)::uuid asset FROM generate_series(1,$1::int)i`, count)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)SELECT media,'movie',990000+i,'Synthetic movie '||lpad(i::text,6,'0'),'2025','{"sortName":"Same Sort","genres":["Drama"],"officialRating":"G"}' FROM native_browse_seed`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT file,$1,'/115/synthetic/'||i||'.mkv',1,'present'FROM native_browse_seed`, f.storage)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,created_at)SELECT asset,file,'2026-01-01T00:00:00Z'::timestamptz FROM native_browse_seed`)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state)SELECT asset,media,'ready'FROM native_browse_seed`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT md5('browse-strm-'||i)::uuid,asset,'Synthetic/'||i||'.strm','synthetic'FROM native_browse_seed`)
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset,'g:test-a','output_directory',1,1 FROM native_browse_seed`)
	// Seeding/ID allocation is outside every timed request. No production mutations.
	f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)SELECT 'movie','tmdb:'||(990000+i)FROM native_browse_seed ON CONFLICT DO NOTHING`)
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,state,result,probed_at)
 SELECT f.id,native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),'ready','{"schema_version":1,"durationTicks":600000000,"container":"mkv","size":12345,"bitRate":123456,"streams":[{"index":0,"type":"Video","codec":"h264","width":1920,"height":1080,"videoRange":"SDR","videoRangeType":"SDR"}]}',now()FROM native_browse_seed s JOIN cloud_files f ON f.id=s.file`)
}
func b5SeedEpisodes(t *testing.T, f browseFixture) {
	t.Helper()
	f.exec(t, `UPDATE native_libraries SET collection_type='tvshows'WHERE id='g:test-b'`)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES(md5('b5-series')::uuid,'series',770000,'Synthetic B5 Series','2025','{"officialRating":"G"}')`)
	f.exec(t, `CREATE TEMP TABLE b5_tv AS SELECT i,((i-1)/1000+1)::int s,((i-1)%1000+1)::int e,md5('b5-tv-file-'||i)::uuid file,md5('b5-tv-asset-'||i)::uuid asset FROM generate_series(1,10000)i`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)SELECT file,$1,'/115/synthetic-tv/'||i||'.mkv',1,'present'FROM b5_tv`, f.storage)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,created_at)SELECT asset,file,'2026-01-01T00:00:00Z'::timestamptz FROM b5_tv`)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state,season,episode)SELECT asset,md5('b5-series')::uuid,'ready',s,e FROM b5_tv`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)SELECT md5('b5-tv-strm-'||i)::uuid,asset,'SyntheticTV/'||i||'.strm','synthetic'FROM b5_tv`)
	f.exec(t, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)SELECT asset,'g:test-b','output_directory',1,1 FROM b5_tv`)
	f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)SELECT 'series','tmdb:770000' UNION SELECT 'season','tmdb:770000:s'||s FROM b5_tv UNION SELECT 'episode','tmdb:770000:s'||s||'e'||e FROM b5_tv ON CONFLICT DO NOTHING`)
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,played,last_played_at)SELECT $1,id,true,now()FROM native_item_ids WHERE internal_key='tmdb:770000:s1e1'`, f.user.ID)
}
func b5Fixture(t *testing.T) (browseFixture, *b5Tracer) {
	f := newBrowseFixture(t)
	if _, err := f.provider.Up(f.ctx); err != nil {
		t.Fatal(err)
	}
	cfg := f.pool.Config()
	tr := &b5Tracer{base: f.trace}
	cfg.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f.pool = pool
	f.repo.DB = pool
	return f, tr
}
func b5Explain(t *testing.T, f browseFixture, tr *b5Tracer, dir, name string) {
	t.Helper()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	tr.mu.Lock()
	queries := append([]b5Query(nil), tr.queries...)
	tr.queries = nil
	tr.explain = true
	tr.mu.Unlock()
	defer func() { tr.mu.Lock(); tr.explain = false; tr.mu.Unlock() }()
	seen := map[string]bool{}
	plans := []json.RawMessage{}
	for _, q := range queries {
		if seen[q.SQL] || (!strings.HasPrefix(q.SQL, "WITH ") && !strings.HasPrefix(q.SQL, "/* native.")) {
			continue
		}
		seen[q.SQL] = true
		var plan json.RawMessage
		explainCtx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
		err := f.pool.QueryRow(explainCtx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+q.SQL, q.Args...).Scan(&plan)
		cancel()
		if err != nil {
			if os.Getenv("B5_BASELINE") == "1" {
				t.Log("B5_BASELINE_EXPLAIN", name, err)
				continue
			}
			t.Fatal("explain", name, err)
		}
		raw, _ := json.Marshal(map[string]any{"sql": q.SQL, "measuredMs": float64(q.Duration) / 1e6, "plan": plan})
		plans = append(plans, raw)
	}
	raw, _ := json.MarshalIndent(plans, "", " ")
	if err := os.WriteFile(filepath.Join(dir, name+"-plans.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestB5NativePerformance(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit reproducible performance workload")
	}
	f, tr := b5Fixture(t)
	b5SeedMovies(t, f, 10000)
	dir := os.Getenv("B5_PROFILE_DIR")
	if dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		Name           string
		Ms             float64
		Bytes, Mallocs uint64
		SQL            int64
	}
	results := []result{}
	run := func(name string, fn func()) {
		t.Helper()
		tr.mu.Lock()
		tr.queries = nil
		tr.mu.Unlock()
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		queries := f.trace.queries.Load()
		start := time.Now()
		fn()
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		r := result{name, float64(elapsed) / 1e6, after.TotalAlloc - before.TotalAlloc, after.Mallocs - before.Mallocs, f.trace.queries.Load() - queries}
		results = append(results, r)
		t.Logf("B5_METRIC %s ms=%.3f allocBytes=%d mallocs=%d sql=%d", r.Name, r.Ms, r.Bytes, r.Mallocs, r.SQL)
		b5Explain(t, f, tr, dir, name)
	}
	q := nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, SortBy: []string{"SortName"}}
	var first string
	run("cold-browse-10000", func() {
		items, n, err := f.repo.Query(f.ctx, f.user, f.permissions(), q)
		if err != nil || n != 10000 || len(items) != 200 {
			t.Fatal(n, err)
		}
		first = items[0].Id
	})
	q.Limit = 24
	run("browse-24", func() {
		_, n, err := f.repo.Query(f.ctx, f.user, f.permissions(), q)
		if err != nil || n != 10000 {
			t.Fatal(n, err)
		}
	})
	run("detail", func() {
		_, n, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{IDs: []string{first}, Limit: 1, Recursive: boolPtr(true)})
		if err != nil || n != 1 {
			t.Fatal(n, err)
		}
	})
	run("latest", func() {
		_, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Latest, Items: q})
		if err != nil {
			t.Fatal(err)
		}
	})
	client := nativeCacheRedis(t)
	cached := catalog.NewCachedBrowse(f.repo, client)
	run("redis-miss", func() {
		_, _, err := cached.Query(f.ctx, f.user, f.permissions(), q)
		if err != nil {
			t.Fatal(err)
		}
	})
	cached = catalog.NewCachedBrowse(f.repo, client)
	run("redis-cold-process-hit", func() {
		_, _, err := cached.Query(f.ctx, f.user, f.permissions(), q)
		if err != nil {
			t.Fatal(err)
		}
	})
	b5SeedEpisodes(t, f)
	var series, season string
	f.pool.QueryRow(f.ctx, `SELECT id::text FROM native_item_ids WHERE internal_key='tmdb:770000'`).Scan(&series)
	f.pool.QueryRow(f.ctx, `SELECT id::text FROM native_item_ids WHERE internal_key='tmdb:770000:s1'`).Scan(&season)
	run("seasons-10000", func() {
		_, n, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: series, IncludeItemTypes: []string{"Season"}, Limit: 200})
		if err != nil && os.Getenv("B5_BASELINE") == "1" {
			t.Log("B5_BASELINE_ERROR seasons", err)
			return
		}
		if err != nil || n != 10 {
			t.Fatal(n, err)
		}
	})
	run("episodes-1000", func() {
		_, n, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: season, IncludeItemTypes: []string{"Episode"}, Limit: 200, SortBy: []string{"IndexNumber"}})
		if err != nil && os.Getenv("B5_BASELINE") == "1" {
			t.Log("B5_BASELINE_ERROR episodes", err)
			return
		}
		if err != nil || n != 1000 {
			t.Fatal(n, err)
		}
	})
	run("nextup-10000", func() {
		_, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.NextUp, Items: nativelibrary.ItemQuery{Limit: 20, Recursive: boolPtr(true)}})
		if err != nil && os.Getenv("B5_BASELINE") == "1" {
			t.Log("B5_BASELINE_ERROR nextup", err)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	})
	if dir != "" {
		cpu, err := os.Create(filepath.Join(dir, "native.cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		before, err := os.Create(filepath.Join(dir, "native.before.alloc.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.Lookup("allocs").WriteTo(before, 0); err != nil {
			t.Fatal(err)
		}
		before.Close()
		if err = pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 8; i++ {
			if _, _, err = f.repo.Query(f.ctx, f.user, f.permissions(), q); err != nil {
				t.Fatal(err)
			}
		}
		pprof.StopCPUProfile()
		cpu.Close()
		runtime.GC()
		for _, kind := range []string{"heap", "allocs"} {
			file, err := os.Create(filepath.Join(dir, "native."+kind+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			if err = pprof.Lookup(kind).WriteTo(file, 0); err != nil {
				t.Fatal(err)
			}
			file.Close()
		}
		raw, _ := json.MarshalIndent(results, "", " ")
		if err = os.WriteFile(filepath.Join(dir, "metrics.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestB5SearchHundredThousand(t *testing.T) {
	if testing.Short() {
		t.Skip("100k regression requires full integration mode")
	}
	f, tr := b5Fixture(t)
	b5SeedMovies(t, f, 100000)
	f.exec(t, `UPDATE media_items SET title='B5 unique late match'WHERE tmdb_id=1089999`)
	start := time.Now()
	out, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 20, IncludeItemTypes: []string{"Movie"}, SearchTerm: "B5 unique late match"}})
	t.Logf("B5_SEARCH_100K duration=%s count=%d err=%v", time.Since(start), len(out.Hints.SearchHints), err)
	if err != nil || len(out.Hints.SearchHints) != 1 {
		t.Fatal("late result missing", err)
	}
	b5Explain(t, f, tr, os.Getenv("B5_PROFILE_DIR"), "search-100000")
	search := func(term string) int {
		t.Helper()
		result, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 20, IncludeItemTypes: []string{"Movie"}, SearchTerm: term}})
		if err != nil {
			t.Fatal(err)
		}
		return len(result.Hints.SearchHints)
	}
	if search("Ｂ５ ｕｎｉｑｕｅ ｌａｔｅ ｍａｔｃｈ") != 1 {
		t.Fatal("NFKC late result lost")
	}
	f.exec(t, `UPDATE media_items SET metadata=metadata||'{"originalTitle":"local-only-hidden-word"}'::jsonb WHERE tmdb_id=1090000`)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,state,payload,fetched_at)VALUES('movie',1090000,'zh-CN','ready','{"originalTitle":"B5 Original 深处"}',now())`)
	if search("B5 Original 深处") != 1 || search("local-only-hidden-word") != 0 {
		t.Fatal("snapshot original title precedence/late result lost")
	}
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1 AND library_id='g:test-a'`, f.user.ID)
	if search("B5 unique late match") != 0 {
		t.Fatal("large search bypassed library revocation")
	}
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'g:test-a',true)`, f.user.ID)
	f.exec(t, `UPDATE media_users SET max_parental_rating=0 WHERE id=$1`, f.user.ID)
	f.exec(t, `UPDATE media_items SET metadata=metadata||'{"officialRating":"R"}'::jsonb WHERE tmdb_id=1089999`)
	if search("B5 unique late match") != 0 {
		t.Fatal("large search bypassed parental filter")
	}
	f.exec(t, `UPDATE media_users SET max_parental_rating=NULL WHERE id=$1`, f.user.ID)
	f.exec(t, `UPDATE cloud_files f SET state='missing'FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id WHERE f.id=a.cloud_file_id AND m.tmdb_id=1089999`)
	if search("B5 unique late match") != 0 {
		t.Fatal("large search exposed missing file")
	}

}
