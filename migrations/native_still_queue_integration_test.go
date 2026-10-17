package migrations_test

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type stillFixture struct {
	browseFixture
	itemID int64
	source movieAsset
	store  *stillqueue.Repository
}

func newStillFixture(t *testing.T) stillFixture {
	t.Helper()
	f := newBrowseFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=true WHERE singleton`)
	f.library(t, "g:still-tv", "tvshows", true, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',250001,'Synthetic Still Show','{}')`, show)
	asset := f.episode(t, show, "g:still-tv", 0, 1, 2)
	if _, err := catalog.PreallocateIDBatch(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	item := f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:250001:s0e1'`)
	store, err := stillqueue.New(f.pool, stillqueue.Config{})
	if err != nil {
		t.Fatal(err)
	}
	out := stillFixture{f, item, asset, store}
	out.probe(t, asset)
	return out
}
func (f stillFixture) probe(t *testing.T, a movieAsset) {
	t.Helper()
	r := probequeue.Repository{DB: f.pool}
	if _, err := r.Enqueue(f.ctx, a.asset); err != nil {
		t.Fatal(err)
	}
	l, err := r.Claim(f.ctx, a.asset, time.Minute)
	if err != nil || l == nil {
		t.Fatal(err)
	}
	ok, err := r.Succeed(f.ctx, *l, probequeue.Result{SchemaVersion: 1, DurationTicks: 615000000})
	if err != nil || !ok {
		t.Fatal(err)
	}
}
func (f stillFixture) enqueue(t *testing.T, force bool) {
	t.Helper()
	var on bool
	var err error
	if force {
		on, err = f.store.Retry(f.ctx, f.itemID)
	} else {
		on, err = f.store.Enqueue(f.ctx, f.itemID)
	}
	if err != nil || !on {
		t.Fatal("enqueue", on, err)
	}
}
func (f stillFixture) claim(t *testing.T) *stillqueue.Lease {
	t.Helper()
	l, err := f.store.Claim(f.ctx, f.itemID)
	if err != nil || l == nil {
		t.Fatal("claim", err)
	}
	a, err := f.store.Admit(f.ctx, *l, 1024)
	if err != nil || !a.Reserved {
		t.Fatal("reserve claimed fixture", a, err)
	}
	return l
}
func stillImage(t *testing.T, l stillqueue.Lease) stillqueue.Image {
	t.Helper()
	path, err := stillqueue.ImagePath(l.AssetID, l.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return stillqueue.Image{RelativePath: path, Digest: strings.Repeat("a", 64), Width: 1280, Height: 720, SizeBytes: 1024}
}
func (f stillFixture) success(t *testing.T, l stillqueue.Lease, want bool) {
	t.Helper()
	if want {
		a, err := f.store.Admit(f.ctx, l, stillImage(t, l).SizeBytes)
		if err != nil || !a.Reserved {
			t.Fatal("reserve fixture image", a, err)
		}
	}
	ok, err := f.store.Succeed(f.ctx, l, stillImage(t, l))
	if err != nil || ok != want {
		t.Fatal("success fence", ok, err)
	}
}
func (f stillFixture) state(t *testing.T) string {
	return f.text(t, `SELECT state FROM media_stills WHERE item_id=$1`, f.itemID)
}

func TestNativeStillAttemptsRetainSnapshotsAndFenceEveryWrite(t *testing.T) {
	f := newStillFixture(t)
	f.enqueue(t, false)
	l := f.claim(t)
	if l.AssetID != f.source.asset || l.DurationTicks != 615000000 || l.Attempt != 1 || l.Until.IsZero() {
		t.Fatal(l)
	}

	if f.integer(t, `SELECT (extract(epoch FROM lease_until-grabbing_started_at)*1000)::bigint FROM media_stills WHERE item_id=$1`, f.itemID) != stillqueue.MaxLease.Milliseconds() {
		t.Fatal("database lease duration exceeded configured cap")
	}
	if later, err := f.store.Claim(f.ctx, f.itemID); err != nil || later != nil {
		t.Fatal("double claim", err)
	}
	f.success(t, *l, true)
	first, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || first == nil || first.Fingerprint != l.Fingerprint {
		t.Fatal(err)
	}
	f.success(t, *l, false)
	if _, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || f.state(t) != "ready" {
		t.Fatal("duplicate reset ready", err)
	}
	for i, class := range []linksource.Class{linksource.Retryable, linksource.Permanent, linksource.Manual} {
		f.enqueue(t, true)
		l = f.claim(t)
		reason := string(linksource.Limited)
		if class == linksource.Permanent {
			reason = string(stillqueue.InvalidImage)
		}
		if class == linksource.Manual {
			reason = string(linksource.Authorization)
		}
		ok, err := f.store.Fail(f.ctx, *l, stillqueue.Failure{Class: class, Reason: reason, RetryAfter: 7 * time.Second})
		if err != nil || !ok {
			t.Fatal(err)
		}
		if f.state(t) != []string{"pending", "failed", "needs_action"}[i] {
			t.Fatal("failure class")
		}
		current, err := f.store.Current(f.ctx, f.itemID)
		if err != nil || current == nil || current.Digest != first.Digest || current.PublishedAt != first.PublishedAt {
			t.Fatal("failure erased old image", err)
		}
		before := f.text(t, `SELECT next_attempt_at::text FROM media_stills WHERE item_id=$1`, f.itemID)
		if _, err := f.store.Enqueue(f.ctx, f.itemID); err != nil {
			t.Fatal(err)
		}
		if f.text(t, `SELECT next_attempt_at::text FROM media_stills WHERE item_id=$1`, f.itemID) != before {
			t.Fatal("duplicate reset backoff")
		}
		if ok, err := f.store.Fail(f.ctx, *l, stillqueue.Failure{Class: linksource.Retryable, Reason: string(stillqueue.ProcessFailed)}); err != nil || ok {
			t.Fatal("late failure", err)
		}
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.still.retry'`) != 3 {
		t.Fatal("manual retry audit")
	}
	f.enqueue(t, true)
	old := f.claim(t)
	f.exec(t, `UPDATE cloud_files SET size_bytes=2 WHERE id=$1`, f.source.file)
	f.success(t, *old, false) // current file fingerprint invalidates a lease even before rescheduling.
	f.probe(t, f.source)
	f.enqueue(t, false)
	next := f.claim(t)
	if old.Fingerprint == next.Fingerprint || old.Token == next.Token {
		t.Fatal("new fingerprint did not replace attempt")
	}
	f.success(t, *old, false)
	stale := *next
	stale.Attempt++
	f.success(t, stale, false)
	if ok, err := f.store.Fail(f.ctx, *old, stillqueue.Failure{Class: linksource.Retryable, Reason: string(linksource.Timeout)}); err != nil || ok {
		t.Fatal("stale failure changed new attempt", err)
	}
	f.success(t, *next, true)
	current, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || current.Fingerprint != next.Fingerprint {
		t.Fatal("new success not published", err)
	}
}

func TestNativeStillCanonicalSourceTMDBAndVisibility(t *testing.T) {
	f := newStillFixture(t)
	f.enqueue(t, false)
	old := f.claim(t)
	// Same episode in another library/version: only the newest probeable asset wins.
	show := f.text(t, `SELECT media_id::text FROM media_asset_bindings WHERE asset_id=$1`, f.source.asset)
	f.library(t, "g:still-other", "tvshows", true, false)
	other := f.episode(t, show, "g:still-other", 0, 1, 1)
	f.exec(t, `UPDATE media_assets SET created_at='2026-02-01' WHERE id=$1`, other.asset)
	// A newer but not-yet-probed version must not displace the eligible source.
	if _, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || f.state(t) != "grabbing" {
		t.Fatal(err)
	}
	f.probe(t, other)
	f.success(t, *old, false) // canonical-source recheck is independent of Enqueue.
	f.enqueue(t, false)
	f.success(t, *old, false)
	l := f.claim(t)
	if l.AssetID != other.asset {
		t.Fatal("wrong canonical source", l.AssetID)
	}
	f.success(t, *l, true)
	// Equal creation times break ties by UUID ascending, exactly like browse.
	f.exec(t, `UPDATE media_assets SET created_at='2026-01-01' WHERE id=$1`, other.asset)
	f.enqueue(t, true)
	l = f.claim(t)
	expected := f.source.asset
	if other.asset < expected {
		expected = other.asset
	}
	if l.AssetID != expected {
		t.Fatal("unstable UUID tie order")
	}
	f.success(t, *l, true)
	// TMDB arriving during a lease prevents a late screenshot from claiming ready.
	f.enqueue(t, true)
	l = f.claim(t)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at)VALUES('season',250001,0,'zh-CN','{"episodes":[{"number":1,"stillPath":"/synthetic.jpg"}]}','ready',now())`)
	f.success(t, *l, false)
	if queued, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || queued || f.state(t) != "skipped" {
		t.Fatal("TMDB not skipped", err)
	}
	if f.text(t, `SELECT last_error_reason FROM media_stills WHERE item_id=$1`, f.itemID) != "tmdb_still" {
		t.Fatal("skip reason")
	}
	if current, err := f.store.Current(f.ctx, f.itemID); err != nil || current == nil {
		t.Fatal("TMDB skip erased GC reference", err)
	}
	f.exec(t, `UPDATE native_tmdb_details SET payload='{}' WHERE tmdb_id=250001`)
	f.enqueue(t, false)
	f.success(t, *f.claim(t), true)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, f.source.file)
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:still-other'`)
	if queued, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || queued || f.state(t) != "skipped" {
		t.Fatal("non-exposable not skipped", err)
	}
	if f.text(t, `SELECT last_error_reason FROM media_stills WHERE item_id=$1`, f.itemID) != "source_unavailable" {
		t.Fatal("source skip")
	}
	// Assets returning restore the same row; trailing episodes are never invented.
	f.exec(t, `UPDATE cloud_files SET state='present' WHERE id=$1`, f.source.file)
	f.enqueue(t, false)
	f.success(t, *f.claim(t), true)
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE internal_key='tmdb:250001:s0e2'`) != 0 {
		t.Fatal("trailing episode created")
	}
	movie := f.media(t, 250002, "Synthetic no still movie", "2026", `{}`)
	f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	key, err := nativelibrary.MovieKey(250002)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Enqueue(f.ctx, ids[key]); !errors.Is(err, stillqueue.ErrNotEligible) {
		t.Fatal("movie eligible", err)
	}
}

func TestNativeStillLeaseRecoveryConcurrencyAndSwitches(t *testing.T) {
	f := newStillFixture(t)
	f.enqueue(t, false)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var claims []*stillqueue.Lease
	var errs []error
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := f.store.Claim(f.ctx, f.itemID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			}
			if l != nil {
				claims = append(claims, l)
			}
		}()
	}
	wg.Wait()
	if len(errs) != 0 || len(claims) != 1 {
		t.Fatal("duplicate claim", len(claims), errs)
	}
	old := *claims[0]
	f.exec(t, `UPDATE media_stills SET grabbing_started_at=now()-interval '6 minutes',lease_until=now()-interval '1 minute' WHERE item_id=$1`, f.itemID)
	n, err := f.store.RecoverExpired(f.ctx)
	if err != nil || n != 1 || f.state(t) != "pending" {
		t.Fatal("lease not recovered", n, err)
	}
	l := f.claim(t)
	if l.Token == old.Token || l.Attempt != 2 {
		t.Fatal("lease fence not advanced")
	}
	f.success(t, old, false)
	f.success(t, *l, true)
	// A busy row is skipped, not waited on.
	f.enqueue(t, true)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, `SELECT item_id FROM media_stills WHERE item_id=$1 FOR UPDATE`, f.itemID); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	claim, err := f.store.Claim(f.ctx, f.itemID)
	if rollbackErr := tx.Rollback(f.ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || claim != nil || time.Since(started) > time.Second {
		t.Fatal("SKIP LOCKED failed", err)
	}
	for _, flag := range []string{"enabled", "probe_enabled", "stills_enabled"} {
		l = f.claim(t)
		f.exec(t, `UPDATE native_server_settings SET `+flag+`=false`)
		if on, err := f.store.Enabled(f.ctx); err != nil || on {
			t.Fatal(flag, err)
		}
		f.success(t, *l, false)
		if _, err := f.store.Enqueue(f.ctx, f.itemID); !errors.Is(err, stillqueue.ErrDisabled) {
			t.Fatal(flag, err)
		}
		if claim, err := f.store.Claim(f.ctx, f.itemID); err != nil || claim != nil {
			t.Fatal(flag, err)
		}
		f.exec(t, `UPDATE media_stills SET grabbing_started_at=now()-interval '6 minutes',lease_until=now()-interval '1 minute' WHERE item_id=$1`, f.itemID)
		if n, err := f.store.RecoverExpired(f.ctx); err != nil || n != 1 {
			t.Fatal("disabled recovery", err)
		}
		f.exec(t, `UPDATE native_server_settings SET `+flag+`=true`)
	}
}

func TestNativeStillMigrationChecksDownUpAndAssetTombstones(t *testing.T) {
	f := newStillFixture(t)
	f.enqueue(t, false)
	l := f.claim(t)
	for _, tt := range []struct{ name, sql string }{
		{"state", `UPDATE media_stills SET state='unknown' WHERE item_id=$1`},
		{"reason", `UPDATE media_stills SET last_error_class='retryable',last_error_reason='raw error secret' WHERE item_id=$1`},
		{"pair", `UPDATE media_stills SET last_error_class='retryable',last_error_reason=NULL WHERE item_id=$1`},
		{"lease cap", `UPDATE media_stills SET lease_until=grabbing_started_at+interval '5 minutes 1 microsecond' WHERE item_id=$1`},
		{"lease missing", `UPDATE media_stills SET lease_token=NULL WHERE item_id=$1`},
		{"ready missing", `UPDATE media_stills SET state='ready',grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL WHERE item_id=$1`},
		{"no duration", `UPDATE media_stills SET duration_ticks=0 WHERE item_id=$1`},
		{"fingerprint", `UPDATE media_stills SET fingerprint='not-hex' WHERE item_id=$1`},
		{"manual class", `UPDATE media_stills SET state='needs_action',grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,last_error_class='permanent',last_error_reason='authorization' WHERE item_id=$1`},
		{"wrong entity", `INSERT INTO media_stills(item_id)SELECT id FROM native_item_ids WHERE entity_type='library' LIMIT 1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "wrong entity" {
				f.rejects(t, "23503", tt.sql)
			} else {
				f.rejects(t, "23514", tt.sql, f.itemID)
			}
		})
	}
	bad := stillImage(t, *l)
	bad.RelativePath = "../escape.jpg"
	if _, err := f.store.Succeed(f.ctx, *l, bad); err != stillqueue.ErrInvalid {
		t.Fatal("unsafe filename")
	}
	bad = stillImage(t, *l)
	bad.Width = 1281
	if _, err := f.store.Succeed(f.ctx, *l, bad); err != stillqueue.ErrInvalid {
		t.Fatal("width")
	}
	if _, err := f.store.Fail(f.ctx, *l, stillqueue.Failure{Class: linksource.Manual, Reason: "https://synthetic.invalid/?secret=NO"}); err != stillqueue.ErrInvalid {
		t.Fatal("unsafe error persisted")
	}
	f.success(t, *l, true)
	f.rejects(t, "23514", `UPDATE media_stills SET relative_path='ab/wrong.jpg' WHERE item_id=$1`, f.itemID)
	f.rejects(t, "23514", `UPDATE media_stills SET result_fingerprint=NULL WHERE item_id=$1`, f.itemID)
	if _, err := f.provider.DownTo(f.ctx, 59); err != nil {
		t.Fatal(err)
	}
	if f.state(t) != "ready" || f.text(t, `SELECT relative_path FROM media_stills WHERE item_id=$1`, f.itemID) != stillImage(t, *l).RelativePath {
		t.Fatal("down lost image")
	}
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	if f.state(t) != "pending" {
		t.Fatal("up must select fresh source")
	}
	legacy, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || legacy == nil || legacy.Fingerprint != "" || legacy.AssetID != "" {
		t.Fatal("invented legacy provenance", err)
	}
	// Metadata-only fixture simulates B2-B's measured legacy-size calibration.
	f.exec(t, `UPDATE media_stills SET size_bytes=1024 WHERE published_at IS NOT NULL`)
	f.enqueue(t, false)
	f.success(t, *f.claim(t), true)
	// SET NULL FKs keep normal asset cleanup from being blocked by cached images.
	f.exec(t, `DELETE FROM strm_entries WHERE asset_id=$1`, f.source.asset)
	f.exec(t, `DELETE FROM native_asset_libraries WHERE asset_id=$1`, f.source.asset)
	f.exec(t, `DELETE FROM media_asset_bindings WHERE asset_id=$1`, f.source.asset)
	f.exec(t, `DELETE FROM media_assets WHERE id=$1`, f.source.asset)
	if f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1 AND asset_id IS NULL AND result_asset_id IS NULL AND published_at IS NOT NULL`, f.itemID) != 1 {
		t.Fatal("asset tombstone lost cache or blocked deletion")
	}
	if queued, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || queued || f.state(t) != "skipped" {
		t.Fatal("deleted asset not skipped", err)
	}
}

func TestNativeStillMigrationNormalizesDirtyLegacyMetadata(t *testing.T) {
	f := newStillFixture(t)
	if _, err := f.provider.DownTo(f.ctx, 59); err != nil {
		t.Fatal(err)
	}
	type check struct{ Name, Definition string }
	var checks []check
	rows, err := f.db.QueryContext(f.ctx, `SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='media_stills'::regclass AND contype='c'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c check
		if err := rows.Scan(&c.Name, &c.Definition); err != nil {
			t.Fatal(err)
		}
		checks = append(checks, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE media_stills DROP CONSTRAINT `+pgx.Identifier{c.Name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		state, path, digest string
		width, height       int
		started, keep       bool
		want                string
	}{
		{"ready", "legacy/valid.jpg", strings.Repeat("a", 64), 1920, 1080, false, true, "pending"},
		{"ready", "", "", 0, 0, false, false, "pending"},
		{"ready", "../../outside", strings.Repeat("b", 64), 1280, 720, false, false, "pending"},
		{"pending", "legacy/valid.jpg", strings.Repeat("c", 64), 100, 100, true, true, "pending"},
		{"failed", "legacy/bad.jpg", "raw secret error", 0, 0, true, false, "failed"},
		{"grabbing", "legacy/valid.jpg", strings.Repeat("d", 64), 100, 100, true, true, "pending"},
		{"skipped", "", "", -1, -1, true, false, "skipped"},
	}
	ids := []int64{}
	for i, c := range cases {
		k, err := nativelibrary.EpisodeKey(250001, 1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		mapping, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{k})
		if err != nil {
			t.Fatal(err)
		}
		id := mapping[k]
		ids = append(ids, id)
		f.exec(t, `INSERT INTO media_stills(item_id,state,relative_path,digest,width,height,grabbing_started_at,error)VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $7 THEN now() END,'SYNTHETIC_PRIVATE_ERROR')`, id, c.state, c.path, c.digest, c.width, c.height, c.started)
	}
	for _, c := range checks {
		if _, err := f.db.ExecContext(f.ctx, `ALTER TABLE media_stills ADD CONSTRAINT `+pgx.Identifier{c.Name}.Sanitize()+` `+c.Definition+` NOT VALID`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal("dirty migration", err)
	}
	for i, c := range cases {
		if f.text(t, `SELECT state FROM media_stills WHERE item_id=$1`, ids[i]) != c.want {
			t.Fatal("dirty state", i)
		}
		s, err := f.store.Current(f.ctx, ids[i])
		if err != nil || (s != nil) != c.keep {
			t.Fatal("dirty snapshot", i, err)
		}
		if f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1 AND grabbing_started_at IS NOT NULL`, ids[i]) != 0 {
			t.Fatal("stale started")
		}
	}
	if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='media_stills' AND column_name='error'`) != 0 {
		t.Fatal("free error column retained")
	}
	if _, err := f.provider.DownTo(f.ctx, 59); err != nil {
		t.Fatal("dirty down", err)
	}
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal("dirty re-up", err)
	}
}

func TestNativeStillConcurrentEnqueueAndBoundedRecovery(t *testing.T) {
	f := newStillFixture(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.Enqueue(f.ctx, f.itemID)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1`, f.itemID) != 1 || f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.still' AND queue='still' AND idempotency_key LIKE 'still:%' AND max_attempts=1`) != 1 {
		t.Fatal("enqueue piled up rows/wakes")
	}
	l := f.claim(t)
	if f.integer(t, `SELECT attempts FROM media_stills WHERE item_id=$1`, f.itemID) != 1 {
		t.Fatal("duplicate enqueue changed attempts")
	}
	// More than one recovery batch, with no runtime processes involved.
	for i := 0; i < 205; i++ {
		key, err := nativelibrary.EpisodeKey(250001, 2, i+1)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, []nativelibrary.Key{key})
		if err != nil {
			t.Fatal(err)
		}
		f.exec(t, `INSERT INTO media_stills(item_id,state,fingerprint,duration_ticks,grabbing_started_at,lease_until,lease_token)
 VALUES($1,'grabbing',$2,1,now()-interval '6 minutes',now()-interval '1 minute',$3)`, ids[key], l.Fingerprint, jobs.UUID())
	}
	n, err := f.store.RecoverExpired(f.ctx)
	if err != nil || n != 200 {
		t.Fatal("unbounded recovery", n, err)
	}
	n, err = f.store.RecoverExpired(f.ctx)
	if err != nil || n != 5 {
		t.Fatal("remaining recovery", n, err)
	}
	if f.state(t) != "grabbing" {
		t.Fatal("recovered nonexpired owner")
	}
	f.success(t, *l, true)
}

func TestNativeStillCandidateDegradesMalformedOptionalMetadata(t *testing.T) {
	f := newStillFixture(t)
	for _, episodes := range []string{`["not-an-int",999999999999999999999]`, `{}`, `null`} {
		f.exec(t, `UPDATE media_asset_bindings SET episodes=$2 WHERE asset_id=$1`, f.source.asset, json.RawMessage(episodes))
		f.enqueue(t, true)
		f.success(t, *f.claim(t), true)
	}
	for _, duration := range []string{`"615000000"`, `-1`, `0`, `1.5`, `999999999999999999999999`} {
		f.exec(t, `UPDATE media_probe_results SET result=jsonb_build_object('durationTicks',$2::jsonb) WHERE cloud_file_id=$1`, f.source.file, json.RawMessage(duration))
		if queued, err := f.store.Enqueue(f.ctx, f.itemID); err != nil || queued || f.state(t) != "skipped" {
			t.Fatal("unsafe duration accepted", duration, err)
		}
	}
}
