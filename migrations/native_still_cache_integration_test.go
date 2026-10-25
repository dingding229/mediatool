package migrations_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func cacheRepository(t *testing.T, f stillFixture, limit int64) *stillqueue.Repository {
	t.Helper()
	r, err := stillqueue.New(f.pool, stillqueue.Config{MaxBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func rawStillClaim(t *testing.T, f stillFixture) stillqueue.Lease {
	t.Helper()
	f.enqueue(t, false)
	l, err := f.store.Claim(f.ctx, f.itemID)
	if err != nil || l == nil {
		t.Fatal("raw claim", err)
	}
	return *l
}
func cacheUsage(t *testing.T, r *stillqueue.Repository) stillqueue.Usage {
	t.Helper()
	u, err := r.Usage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func seedStillSnapshots(t *testing.T, f stillFixture, n int, size int64) []stillqueue.Snapshot {
	t.Helper()
	var keys []nativelibrary.Key
	for i := 0; i < n; i++ {
		k, err := nativelibrary.EpisodeKey(251002, 1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, keys)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []stillqueue.Snapshot
	for _, k := range keys {
		asset := jobs.UUID()
		fp := strings.Repeat("b", 64)
		path, err := stillqueue.ImagePath(asset, fp)
		if err != nil {
			t.Fatal(err)
		}
		f.exec(t, `INSERT INTO media_stills(item_id,state,fingerprint,result_fingerprint,relative_path,digest,width,height,size_bytes,published_at,last_access_at)
 VALUES($1,'ready',$2,$2,$3,$4,1280,720,$5,clock_timestamp()-interval '10 days',clock_timestamp()-interval '2 days')`, ids[k], fp, path, strings.Repeat("a", 64), size)
		s, err := f.store.Current(f.ctx, ids[k])
		if err != nil || s == nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, *s)
	}
	return snapshots
}
func addStillEpisode(t *testing.T, f stillFixture, number int) stillFixture {
	t.Helper()
	show := f.text(t, `SELECT media_id::text FROM media_asset_bindings WHERE asset_id=$1`, f.source.asset)
	a := f.episode(t, show, "g:still-tv", 0, number, number)
	if _, err := catalog.PreallocateIDBatch(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	key, err := nativelibrary.EpisodeKey(250001, 0, number)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{key})
	if err != nil {
		t.Fatal(err)
	}
	out := stillFixture{f.browseFixture, ids[key], a, f.store}
	out.probe(t, a)
	return out
}

func TestNativeStillCacheReservationsFenceAndCrashRecovery(t *testing.T) {
	f := newStillFixture(t)
	f.store = cacheRepository(t, f, 2048)
	l := rawStillClaim(t, f)
	image := stillImage(t, l)
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || ok {
		t.Fatal("success bypassed admission", ok, err)
	}
	for i := 0; i < 3; i++ {
		a, err := f.store.Admit(f.ctx, l, 1024)
		if err != nil || !a.Reserved || len(a.Evicted) != 0 {
			t.Fatal(a, err)
		}
		if u := cacheUsage(t, f.store); u.SnapshotBytes != 0 || u.ReservedBytes != 1024 {
			t.Fatal("duplicate reservation", u)
		}
	}
	wrong := l
	wrong.Attempt++
	if _, err := f.store.Admit(f.ctx, wrong, 1024); !errors.Is(err, stillqueue.ErrLeaseLost) {
		t.Fatal("stale admission", err)
	}
	for _, bad := range []int64{0, -1, stillqueue.MaxImageBytes + 1} {
		if _, err := f.store.Admit(f.ctx, l, bad); err != stillqueue.ErrInvalid {
			t.Fatal("bad size", err)
		}
	}
	image.SizeBytes = 2048
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || ok {
		t.Fatal("receipt mismatched reservation", ok, err)
	}
	image.SizeBytes = 1024
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if u := cacheUsage(t, f.store); u.SnapshotBytes != 1024 || u.ReservedBytes != 0 {
		t.Fatal("not atomically converted", u)
	}
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	next := rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, next, 1024); err != nil || !a.Reserved {
		t.Fatal(a, err)
	}
	// Crash leaves a stored reservation. Advancing BOTH lease timestamps keeps
	// the <=5 minute CHECK valid, without waiting wall-clock minutes in tests.
	f.exec(t, `UPDATE media_stills SET grabbing_started_at=now()-interval '6 minutes',lease_until=now()-interval '1 minute' WHERE item_id=$1`, f.itemID)
	if u := cacheUsage(t, f.store); u.ReservedBytes != 0 || u.SnapshotBytes != 1024 {
		t.Fatal("expired reservation occupies quota", u)
	}
	if stored := f.integer(t, `SELECT reserved_bytes FROM media_stills WHERE item_id=$1`, f.itemID); stored != 1024 {
		t.Fatal("fixture did not leak", stored)
	}
	if n, err := f.store.RecoverExpired(f.ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n := f.integer(t, `SELECT reserved_bytes FROM media_stills WHERE item_id=$1`, f.itemID); n != 0 {
		t.Fatal("reservation leaked after recovery", n)
	}
	if old, err := f.store.Current(f.ctx, f.itemID); err != nil || old == nil || old.SizeBytes != 1024 {
		t.Fatal("crash erased old snapshot", err)
	}
	if ok, err := f.store.Succeed(f.ctx, next, image); err != nil || ok {
		t.Fatal("expired worker wrote back", ok, err)
	}
	if _, err := f.store.Admit(f.ctx, next, 1024); !errors.Is(err, stillqueue.ErrLeaseLost) {
		t.Fatal(err)
	}
	resumed := rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, resumed, 1024); err != nil || !a.Reserved {
		t.Fatal("recovered quota not reusable", a, err)
	}
	if ok, err := f.store.Succeed(f.ctx, next, image); err != nil || ok {
		t.Fatal("old worker consumed new reservation", ok, err)
	}
	if ok, err := f.store.Fail(f.ctx, resumed, stillqueue.Failure{Class: linksource.Retryable, Reason: string(linksource.Timeout)}); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if u := cacheUsage(t, f.store); u.ReservedBytes != 0 || u.SnapshotBytes != 1024 {
		t.Fatal("failure leaked reservation", u)
	}
}

func TestNativeStillCacheLRULowWaterAndFencedEviction(t *testing.T) {
	f := newStillFixture(t)
	f.store = cacheRepository(t, f, 10000)
	snapshots := seedStillSnapshots(t, f, 10, 1000)
	// A hot image moves behind cold images; no filesystem atime participates.
	if n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{{ItemID: snapshots[0].ItemID, RelativePath: snapshots[0].RelativePath, Digest: snapshots[0].Digest}}); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	l := rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, l, 10001); err != nil || a.Reserved || len(a.Evicted) != 0 {
		t.Fatal("impossible image drained cache", a, err)
	}
	a, err := f.store.Admit(f.ctx, l, 1024)
	if err != nil || !a.Reserved || len(a.Evicted) != 2 || a.Evicted[0].ItemID != snapshots[1].ItemID || a.Evicted[1].ItemID != snapshots[2].ItemID {
		t.Fatal("LRU selection", a, err)
	}
	if u := cacheUsage(t, f.store); u.SnapshotBytes != 8000 || u.ReservedBytes != 1024 {
		t.Fatal("wrong low-water usage", u)
	}
	for _, s := range a.Evicted {
		if current, err := f.store.Current(f.ctx, s.ItemID); err != nil || current != nil {
			t.Fatal("uncommitted eviction returned", err)
		}
		if n := f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1 AND state='skipped' AND last_error_reason='cache_evicted' AND evicted_at IS NOT NULL AND reserved_bytes=0`, s.ItemID); n != 1 {
			t.Fatal("not cache evicted")
		}
		if ok, err := f.store.EvictSnapshot(f.ctx, s); err != nil || ok {
			t.Fatal("duplicate eviction", ok, err)
		}
	}
	observed := snapshots[0]
	observed.PublishedAt = observed.PublishedAt.Add(-time.Second)
	if ok, err := f.store.EvictSnapshot(f.ctx, observed); err != nil || ok {
		t.Fatal("stale snapshot fence ignored", ok, err)
	}
	// Eviction of an observed old snapshot invalidates an in-flight replacement.
	image := stillImage(t, l)
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	current, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || current == nil {
		t.Fatal(err)
	}
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	l = rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, l, 1024); err != nil || !a.Reserved {
		t.Fatal(a, err)
	}
	if ok, err := f.store.EvictSnapshot(f.ctx, *current); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || ok {
		t.Fatal("evicted lease republished", ok, err)
	}
	if n := f.integer(t, `SELECT reserved_bytes FROM media_stills WHERE item_id=$1`, f.itemID); n != 0 {
		t.Fatal("eviction leaked reservation")
	}
}

func TestNativeStillCacheBoundedEvictionsAndUnknownLegacySizes(t *testing.T) {
	f := newStillFixture(t)
	f.store = cacheRepository(t, f, 100)
	snapshots := seedStillSnapshots(t, f, 450, 10)
	l := rawStillClaim(t, f)
	for round, want := range []int{200, 200, 41} {
		a, err := f.store.Admit(f.ctx, l, 10)
		if err != nil || len(a.Evicted) != want || a.Reserved != (round == 2) {
			t.Fatal("bounded partial progress", round, a, err)
		}
	}
	if u := cacheUsage(t, f.store); u.SnapshotBytes != 90 || u.ReservedBytes != 10 {
		t.Fatal(u)
	}
	// Unknown legacy sizes are not silently counted as free disk space.
	f.exec(t, `UPDATE media_stills SET size_bytes=0 WHERE item_id=$1`, snapshots[449].ItemID)
	if _, err := f.store.Admit(f.ctx, l, 10); !errors.Is(err, stillqueue.ErrUnknownSize) {
		t.Fatal("unknown size admitted", err)
	}
	if u := cacheUsage(t, f.store); u.UnknownSizes != 1 || u.ReservedBytes != 10 {
		t.Fatal(u)
	}
}

func TestNativeStillCacheConcurrentAdmissionAndLockBudgets(t *testing.T) {
	f := newStillFixture(t)
	f.store = cacheRepository(t, f, 1024)
	other := addStillEpisode(t, f, 3)
	leases := []stillqueue.Lease{rawStillClaim(t, f), rawStillClaim(t, other)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []stillqueue.Admission
	var errs []error
	for _, l := range leases {
		wg.Add(1)
		go func(l stillqueue.Lease) {
			defer wg.Done()
			a, err := f.store.Admit(f.ctx, l, 1024)
			mu.Lock()
			defer mu.Unlock()
			results = append(results, a)
			errs = append(errs, err)
		}(l)
	}
	wg.Wait()
	winners := 0
	for i, a := range results {
		if errs[i] != nil && !errors.Is(errs[i], stillqueue.ErrCacheBusy) {
			t.Fatal(errs[i])
		}
		if a.Reserved {
			winners++
		}
	}
	if winners != 1 || cacheUsage(t, f.store).ReservedBytes != 1024 {
		t.Fatal("overcommitted concurrent quota", winners)
	}
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := blocker.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := blocker.Exec(f.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('media-tool:native-still-quota',0))`); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := f.store.Admit(f.ctx, leases[0], 1024); !errors.Is(err, stillqueue.ErrCacheBusy) {
		t.Fatal("global lock waited", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("lock exceeded budget")
	}
	if err := blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	blocker, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(f.ctx, `SELECT item_id FROM media_stills WHERE item_id=$1 FOR UPDATE`, leases[0].ItemID); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(f.ctx, 50*time.Millisecond)
	defer cancel()
	started = time.Now()
	_, err = f.store.Admit(short, leases[0], 1024)
	if err == nil || time.Since(started) > time.Second {
		t.Fatal("row lock ignored request budget", err)
	}
}

func TestNativeStillCacheTouchBatchThrottleAndFence(t *testing.T) {
	f := newStillFixture(t)
	s := seedStillSnapshots(t, f, 1, 1024)[0]
	access := stillqueue.Access{ItemID: s.ItemID, RelativePath: s.RelativePath, Digest: s.Digest}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var writes int64
	var errs []error
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{access, access})
			mu.Lock()
			writes += n
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 {
		t.Fatal("more than one write per hour", writes)
	}
	stamp := f.text(t, `SELECT last_access_at::text FROM media_stills WHERE item_id=$1`, s.ItemID)
	if n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{access}); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if f.text(t, `SELECT last_access_at::text FROM media_stills WHERE item_id=$1`, s.ItemID) != stamp {
		t.Fatal("throttled touch wrote row")
	}
	f.exec(t, `UPDATE media_stills SET last_access_at=clock_timestamp()-interval '61 minutes' WHERE item_id=$1`, s.ItemID)
	bad := access
	bad.Digest = strings.Repeat("c", 64)
	if n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{bad}); err != nil || n != 0 {
		t.Fatal("stale image touch", n, err)
	}
	bad = access
	bad.RelativePath = "../other.jpg"
	if n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{bad}); err != nil || n != 0 {
		t.Fatal("wrong image touch", n, err)
	}
	if n, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{access}); err != nil || n != 1 {
		t.Fatal("next hour", n, err)
	}
	if n, err := f.store.TouchBatch(f.ctx, nil); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err := f.store.TouchBatch(f.ctx, make([]stillqueue.Access, 201)); err != stillqueue.ErrInvalid {
		t.Fatal("unbounded batch", err)
	}
	if _, err := f.store.TouchBatch(f.ctx, []stillqueue.Access{{ItemID: 1, Digest: access.Digest}}); err != stillqueue.ErrInvalid {
		t.Fatal(err)
	}
}

func TestNativeStillCacheEvictionCooldownRequeuesOnce(t *testing.T) {
	f := newStillFixture(t)
	l := rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, l, 1024); err != nil || !a.Reserved {
		t.Fatal(a, err)
	}
	f.success(t, l, true)
	s, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if ok, err := f.store.EvictSnapshot(f.ctx, *s); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := f.store.RequeueEvicted(f.ctx, f.itemID); err != nil || ok {
		t.Fatal("cooldown bypassed", ok, err)
	}
	f.exec(t, `UPDATE media_stills SET evicted_at=clock_timestamp()-interval '23 hours 59 minutes' WHERE item_id=$1`, f.itemID)
	if ok, err := f.store.RequeueEvicted(f.ctx, f.itemID); err != nil || ok {
		t.Fatal("early", ok, err)
	}
	f.exec(t, `UPDATE media_stills SET evicted_at=clock_timestamp()-interval '24 hours 1 second' WHERE item_id=$1`, f.itemID)
	if ok, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || ok {
		t.Fatal("ordinary sweep revived evicted image", ok, err)
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=false`)
	if _, err := f.store.RequeueEvicted(f.ctx, f.itemID); !errors.Is(err, stillqueue.ErrDisabled) {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true`)
	f.exec(t, `UPDATE jobs SET state='succeeded' WHERE kind='native.still'`)
	var wg sync.WaitGroup
	var mu sync.Mutex
	queued := 0
	var errs []error
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := f.store.RequeueEvicted(f.ctx, f.itemID)
			mu.Lock()
			if ok {
				queued++
			}
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if queued != 1 || f.state(t) != "pending" {
		t.Fatal("cooldown did not merge", queued, f.state(t))
	}
	if n := f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.still' AND state='pending'`); n != 1 {
		t.Fatal("duplicate wakeups", n)
	}
	if n := f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.still.retry'`); n != 0 {
		t.Fatal("automatic recovery impersonated admin")
	}
	if ok, err := f.store.RequeueEvicted(f.ctx, f.itemID); err != nil || ok {
		t.Fatal("duplicate request", ok, err)
	}
}

func TestNativeStillCacheReservationReleasedOnAllTransitions(t *testing.T) {
	f := newStillFixture(t)
	reserve := func() stillqueue.Lease {
		t.Helper()
		if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
			t.Fatal(err)
		}
		l := rawStillClaim(t, f)
		if a, err := f.store.Admit(f.ctx, l, 1024); err != nil || !a.Reserved {
			t.Fatal(a, err)
		}
		return l
	}
	released := func() {
		t.Helper()
		if n := f.integer(t, `SELECT reserved_bytes FROM media_stills WHERE item_id=$1`, f.itemID); n != 0 {
			t.Fatal("transition leaked reservation", n)
		}
	}
	for _, class := range []linksource.Class{linksource.Permanent, linksource.Manual, linksource.Retryable} {
		l := reserve()
		if ok, err := f.store.Fail(f.ctx, l, stillqueue.Failure{Class: class, Reason: string(stillqueue.StorageUnavailable)}); err != nil || !ok {
			t.Fatal(ok, err)
		}
		released()
	}
	old := reserve()
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	released()
	if ok, err := f.store.Succeed(f.ctx, old, stillImage(t, old)); err != nil || ok {
		t.Fatal("retry resurrected old lease", ok, err)
	}
	old = reserve()
	f.exec(t, `UPDATE cloud_files SET size_bytes=size_bytes+1 WHERE id=$1`, f.source.file)
	f.probe(t, f.source)
	f.enqueue(t, false)
	released()
	old = reserve()
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at)
 VALUES('season',250001,0,'zh-CN','{"episodes":[{"number":1,"stillPath":"/synthetic.jpg"}]}','ready',now())`)
	if ok, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || ok {
		t.Fatal(ok, err)
	}
	released()
	if ok, err := f.store.Succeed(f.ctx, old, stillImage(t, old)); err != nil || ok {
		t.Fatal("TMDB skip reused old lease", ok, err)
	}
}

func TestNativeStillCacheSchemaBoundsAndRoundTrip(t *testing.T) {
	f := newStillFixture(t, 60)
	l := rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, l, 1024); err != nil || !a.Reserved {
		t.Fatal(a, err)
	}
	f.exec(t, `UPDATE media_stills SET reserved_bytes=$2 WHERE item_id=$1`, f.itemID, stillqueue.MaxImageBytes)
	f.rejects(t, "23514", `UPDATE media_stills SET reserved_bytes=$2 WHERE item_id=$1`, f.itemID, stillqueue.MaxImageBytes+1)
	f.rejects(t, "23514", `UPDATE media_stills SET reserved_bytes=-1 WHERE item_id=$1`, f.itemID)
	f.rejects(t, "23514", `UPDATE media_stills SET size_bytes=1 WHERE item_id=$1`, f.itemID)
	f.rejects(t, "23514", `UPDATE media_stills SET state='skipped',grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,last_error_class='permanent',last_error_reason='source_unavailable' WHERE item_id=$1`, f.itemID)
	f.success(t, l, true)
	f.exec(t, `UPDATE media_stills SET size_bytes=$2 WHERE item_id=$1`, f.itemID, stillqueue.MaxImageBytes)
	f.rejects(t, "23514", `UPDATE media_stills SET size_bytes=$2 WHERE item_id=$1`, f.itemID, stillqueue.MaxImageBytes+1)
	f.rejects(t, "23514", `UPDATE media_stills SET reserved_bytes=1 WHERE item_id=$1`, f.itemID)
	f.rejects(t, "23514", `UPDATE media_stills SET last_error_class='permanent',last_error_reason='cache_evicted' WHERE item_id=$1`, f.itemID)
	// Down can discard a live reservation without leaving a CHECK conflict.
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	l = rawStillClaim(t, f)
	if a, err := f.store.Admit(f.ctx, l, 1024); err != nil || !a.Reserved {
		t.Fatal(a, err)
	}
	if _, err := f.provider.DownTo(f.ctx, 59); err != nil {
		t.Fatal("down with reservation", err)
	}
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal("re-up", err)
	}
	u := cacheUsage(t, f.store)
	if u.ReservedBytes != 0 || u.UnknownSizes != 1 {
		t.Fatal("roundtrip invented size or retained reservation", u)
	}
	if _, err := f.store.Admit(f.ctx, l, 1024); !errors.Is(err, stillqueue.ErrLeaseLost) {
		t.Fatal("old lease survived roundtrip", err)
	}
	for _, index := range []string{"media_stills_cache_size", "media_stills_reservations", "media_stills_evicted"} {
		if n := f.integer(t, `SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, index); n != 1 {
			t.Fatal("cache index missing", index)
		}
	}
}

func TestNativeStillCacheFailedCommitNeverAuthorizesFileDeletion(t *testing.T) {
	f := newStillFixture(t)
	f.store = cacheRepository(t, f, 1500)
	snapshot := seedStillSnapshots(t, f, 1, 1000)[0]
	l := rawStillClaim(t, f)
	// DDL legitimately reports zero affected rows; do not use the DML fixture helper.
	for _, ddl := range []string{
		`CREATE FUNCTION reject_cache_eviction_commit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic deferred rejection'; END;$$`,
		`CREATE CONSTRAINT TRIGGER reject_cache_eviction AFTER UPDATE ON media_stills DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW WHEN(NEW.last_error_reason='cache_evicted') EXECUTE FUNCTION reject_cache_eviction_commit()`,
	} {
		if _, err := f.pool.Exec(f.ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	a, err := f.store.Admit(f.ctx, l, 1024)
	if err == nil || a.Reserved || len(a.Evicted) != 0 {
		t.Fatal("failed commit authorized deletion", a, err)
	}
	if current, err := f.store.Current(f.ctx, snapshot.ItemID); err != nil || current == nil || current.Digest != snapshot.Digest {
		t.Fatal("rollback lost reference", err)
	}
	if u := cacheUsage(t, f.store); u.SnapshotBytes != 1000 || u.ReservedBytes != 0 {
		t.Fatal("failed commit left quota writes", u)
	}
	if ok, err := f.store.EvictSnapshot(f.ctx, snapshot); err == nil || ok {
		t.Fatal("failed eviction commit authorized deletion", ok, err)
	}
}
