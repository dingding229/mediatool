package migrations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func nativeCacheRedis(t *testing.T) *redis.Client {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("Redis DB15 required")
	}
	opt, err := redis.ParseURL(raw)
	if err != nil || opt.DB != 15 {
		t.Fatal("only isolated Redis DB15 is permitted")
	}
	opt.ContextTimeoutEnabled = true
	opt.MaxRetries = 0
	opt.DialTimeout = time.Second
	opt.ReadTimeout = time.Second
	opt.WriteTimeout = time.Second
	r := redis.NewClient(opt)
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := r.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestNativeCacheTenThousandColdProcessAndWriteFences(t *testing.T) {
	f := newBrowseFixture(t)
	seedBrowseMovies(t, f, 10000)
	client := nativeCacheRedis(t)
	diag := &browseRedisTrace{}
	client.AddHook(diag)
	r := catalog.NewCachedBrowse(f.repo, client)
	q := nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, SortBy: []string{"SortName"}}
	start := time.Now()
	items, total, err := r.Query(f.ctx, f.user, f.permissions(), q)
	miss := time.Since(start)
	if err != nil || total != 10000 || len(items) != 200 {
		t.Fatal("cache miss", total, len(items), err)
	}
	var visibility string
	if e := f.pool.QueryRow(f.ctx, `SELECT pg_current_snapshot()::text`).Scan(&visibility); e != nil {
		t.Fatal(e)
	}
	f.trace.queries.Store(0)
	probes := f.trace.probes.Load()
	// New repository = cold process memory, Redis survives it. No DB content warmup
	// is hidden in the stopwatch; first-ever Redis miss is reported independently.
	r = catalog.NewCachedBrowse(f.repo, client)
	start = time.Now()
	cached, n, err := r.Query(f.ctx, f.user, f.permissions(), q)
	cold := time.Since(start)
	sql := f.trace.queries.Load()
	t.Logf("T53 10000 movies: first-ever Redis miss=%s cold-repository Redis hit=%s SQL=%d", miss, cold, sql)
	if err != nil || n != 10000 || len(cached) != 200 || cached[0].Id != items[0].Id || sql > 3 || f.trace.probes.Load() != probes || cold >= 200*time.Millisecond || cold >= miss {
		t.Log("MVCC after fill", visibility, "Redis command metadata", diag.snapshot())
		t.Fatal("cold-process cache/performance", err, cold, miss, sql)
	}
	id := items[0].Id
	// No cross-request authorization cache, including direct SQL grants changes.
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1`, f.user.ID)
	none, n, err := r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 0 || len(none) != 0 {
		t.Fatal("stale grants cache", err, n)
	}
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'g:test-a',true)`, f.user.ID)
	patch := nativelibrary.PersonalPatch{Favorite: boolPtr(true)}
	if _, err := r.PersonalData(f.ctx, f.user, f.permissions(), id, &patch); err != nil {
		t.Fatal(err)
	}
	items, n, err = r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 10000 || !items[0].UserData.IsFavorite {
		t.Fatal("user-data write did not invalidate", err)
	}
	f.exec(t, `UPDATE media_probe_results SET result=jsonb_set(result,'{durationTicks}','900000000') WHERE cloud_file_id=(SELECT a.cloud_file_id FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id JOIN native_item_ids i ON i.internal_key='tmdb:'||m.tmdb_id WHERE i.id=$1::bigint LIMIT 1)`, id)
	items, _, err = r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || items[0].RunTimeTicks == nil || *items[0].RunTimeTicks != 900000000 {
		t.Fatal("probe write did not invalidate", err)
	}
	f.exec(t, `UPDATE media_items SET metadata=metadata||'{"officialRating":"R"}'::jsonb WHERE media_type='movie'`)
	f.exec(t, `UPDATE media_users SET max_parental_rating=0 WHERE id=$1`, f.user.ID)
	items, n, err = r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 0 || len(items) != 0 {
		t.Fatal("stale parental scope", err, n)
	}
	f.exec(t, `UPDATE media_users SET max_parental_rating=NULL WHERE id=$1`, f.user.ID)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=(SELECT a.cloud_file_id FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id JOIN native_item_ids i ON i.internal_key='tmdb:'||m.tmdb_id WHERE i.id=$1::bigint LIMIT 1)`, id)
	items, n, err = r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 9999 || items[0].Id == id {
		t.Fatal("stale exposability cache", err, n)
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	items, n, err = r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 0 || len(items) != 0 {
		t.Fatal("stale hidden library", err, n)
	}
	f.exec(t, `UPDATE media_users SET enabled=false WHERE id=$1`, f.user.ID)
	if _, _, err = r.Query(f.ctx, f.user, f.permissions(), q); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("disabled user cached", err)
	}
}
func TestNativeCacheIndependentResponsesAndRedisFallback(t *testing.T) {
	f := newBrowseFixture(t)
	client := nativeCacheRedis(t)
	m := f.media(t, 981053, "Synthetic Cache Movie", "2025", `{"officialRating":"G"}`)
	f.asset(t, m, "g:test-a", time.Now(), "mkv")
	f.lifecycle(t)
	q := nativelibrary.ItemQuery{Limit: 1, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}}
	r := catalog.NewCachedBrowse(f.repo, client)
	if _, _, err := r.Query(f.ctx, f.user, f.permissions(), q); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	fail := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			items, n, err := r.Query(f.ctx, f.user, f.permissions(), q)
			if err == nil && (n != 1 || items[0].Type != dto.Movie) {
				err = errors.New("invalid cache page")
			}
			if err == nil {
				items[0].UserData.Key = "local-only"
			}
			fail <- err
		})
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		if err != nil {
			t.Fatal(err)
		}
	}
	// A transaction that already has an older xid can commit after a cache
	// fill: xmax alone would miss it, so the in-progress set is part of the key.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	initial, _, err := r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `INSERT INTO media_user_item_data(user_id,item_id,favorite)VALUES($1,$2::bigint,true) ON CONFLICT(user_id,item_id)DO UPDATE SET favorite=true`, f.user.ID, initial[0].Id); err != nil {
		t.Fatal(err)
	}
	before, _, err := r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || before[0].UserData.IsFavorite {
		t.Fatal("uncommitted data exposed", err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, _, err := r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || !after[0].UserData.IsFavorite {
		t.Fatal("late old-xid commit missed", err)
	}
	views, err := r.Views(f.ctx, f.user, f.permissions())
	if err != nil || len(views) == 0 {
		t.Fatal("views", err)
	}
	counts, err := r.Counts(f.ctx, f.user, f.permissions())
	if err != nil || counts.MovieCount != 1 {
		t.Fatal("counts", err)
	}
	result, err := r.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Latest, Items: q})
	if err != nil || len(result.Items) != 1 {
		t.Fatal("discovery", err)
	}
	// Poison only this test schema's keys, identified by the newly-created key
	// set (DB15 is isolated and the suite is serial). Use a bounded SCAN, never KEYS.
	var cursor uint64
	var keys []string
	for i := 0; i < 20; i++ {
		batch, next, err := client.Scan(f.ctx, cursor, "native:browse:v2:*", 100).Result()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	for _, key := range keys {
		ttl, err := client.PTTL(f.ctx, key).Result()
		if err != nil || ttl > catalog.BrowseCacheTTL || ttl <= 0 {
			t.Fatal("unbounded TTL", ttl, err)
		}
		value, err := client.Get(f.ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(value, "Synthetic Cache Movie") {
			if err = client.Set(f.ctx, key, strings.Repeat("x", 3<<20), time.Second).Err(); err != nil {
				t.Fatal(err)
			}
		}
	}
	items, n, err := r.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || n != 1 || len(items) != 1 || items[0].UserData.Key == "local-only" {
		t.Fatal("poisoned/oversize cache returned", err)
	}
	// Separate closed client: source SQL still works and does not cache failure.
	opt := *client.Options()
	offline := redis.NewClient(&opt)
	if err := offline.Close(); err != nil {
		t.Fatal(err)
	}
	r = catalog.NewCachedBrowse(f.repo, offline)
	if _, n, err := r.Query(f.ctx, f.user, f.permissions(), q); err != nil || n != 1 {
		t.Fatal("Redis degradation", err)
	}
}
func TestNativeDiscoveryCandidateCapAfterSearch(t *testing.T) {
	f := newBrowseFixture(t)
	seedBrowseMovies(t, f, 50001)
	f.exec(t, `UPDATE media_items SET title='独特 ＣＡＣＨＥ',metadata=metadata||'{"originalTitle":"Unique Narrow Target"}'::jsonb WHERE tmdb_id=1040001`)
	for _, v := range []struct {
		mode nativelibrary.DiscoveryMode
		term string
	}{{nativelibrary.Hints, "unique narrow"}, {nativelibrary.Hints, "cache"}, {nativelibrary.Genres, "独特"}, {nativelibrary.Filters, "独特"}} {
		out, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: v.mode, Items: nativelibrary.ItemQuery{Limit: 10, IncludeItemTypes: []string{"Movie"}, SearchTerm: v.term}})
		if err != nil || v.mode == nativelibrary.Hints && out.Hints.TotalRecordCount != 1 {
			t.Fatal("selective search rejected large library", v, err, out.Total)
		}
	}
	_, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 10, IncludeItemTypes: []string{"Movie"}, SearchTerm: "Synthetic"}})
	// The unique title replaces one movie, leaving exactly 50000 matches: allowed.
	if err != nil {
		t.Fatal("exactly-50000 boundary", err)
	}
	f.exec(t, `UPDATE media_items SET title='Synthetic Unique Target' WHERE tmdb_id=1040001`)
	_, err = f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{Limit: 10, IncludeItemTypes: []string{"Movie"}, SearchTerm: "Synthetic"}})
	if !errors.Is(err, nativelibrary.ErrDiscoveryTooBroad) {
		t.Fatal("broad search did not explicitly reject", err)
	}
}

// Command names/key digests/errors only: never cache payloads or credentials.
type browseRedisTrace struct {
	mu     sync.Mutex
	events []string
}

func (d *browseRedisTrace) DialHook(next redis.DialHook) redis.DialHook { return next }
func (d *browseRedisTrace) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (d *browseRedisTrace) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, c redis.Cmder) error {
		e := next(ctx, c)
		if c.Name() == "set" || c.Name() == "eval" {
			key := ""
			if len(c.Args()) > 1 {
				key = fmt.Sprint(c.Args()[1])
				if c.Name() == "eval" && len(c.Args()) > 3 {
					key = fmt.Sprint(c.Args()[3])
				}
			}
			d.mu.Lock()
			d.events = append(d.events, c.Name()+":"+key+":"+fmt.Sprint(e))
			d.mu.Unlock()
		}
		return e
	}
}
func (d *browseRedisTrace) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.events...)
}
