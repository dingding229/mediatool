package migrations_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
)

func stillJPEG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func stillFileCache(t *testing.T, f stillFixture) (*stillstore.Store, *stillstore.Cache, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "native-media", "stills")
	store, err := stillstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := stillstore.NewCache(store, f.store)
	if err != nil {
		if e := store.Close(); e != nil {
			t.Error(e)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	})
	return store, cache, root
}
func writeSeedStill(t *testing.T, f stillFixture, s *stillstore.Store, snapshot stillqueue.Snapshot, raw []byte) stillqueue.Snapshot {
	t.Helper()
	asset := snapshot.RelativePath[3:39]
	image, err := s.Publish(f.ctx, asset, strings.Repeat("b", 64), raw)
	if err != nil {
		t.Fatal(err)
	} // Synthetic fixture only; runtime uses Cache.
	f.exec(t, `UPDATE media_stills SET digest=$2 WHERE item_id=$1`, snapshot.ItemID, image.Digest)
	current, err := f.store.Current(f.ctx, snapshot.ItemID)
	if err != nil || current == nil {
		t.Fatal(err)
	}
	return *current
}
func TestNativeStillStoreUnknownUnstatableSnapshotDoesNotBlockNewFrame(t *testing.T) {
	f := newStillFixture(t)
	raw := stillJPEG(t)
	store, cache, root := stillFileCache(t, f)
	old := seedStillSnapshots(t, f, 1, 0)[0]
	old = writeSeedStill(t, f, store, old, raw)
	outside := filepath.Join(t.TempDir(), "synthetic-sentinel")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, old.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, old.RelativePath)); err != nil {
		t.Fatal(err)
	}
	l := rawStillClaim(t, f)
	image, err := cache.Publish(f.ctx, l, raw)
	if err != nil {
		t.Fatal("one failed stat blocked new capture", err)
	}
	if current, err := f.store.Current(f.ctx, old.ItemID); err != nil || current != nil {
		t.Fatal("bad reference not cleared", err)
	}
	if n := f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1 AND state='skipped' AND last_error_reason='cache_evicted'`, old.ItemID); n != 1 {
		t.Fatal("not evicted")
	}
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if u := cacheUsage(t, f.store); u.UnknownSizes != 0 || u.SnapshotBytes != int64(len(raw)) || u.ReservedBytes != 0 {
		t.Fatal(u)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatal("followed symlink", err)
	}
	if cache.CleanupFailures() == 0 {
		t.Fatal("unlink failure not recorded")
	}
}
func TestNativeStillStoreMissingOwnLegacySnapshotPreservesLiveLease(t *testing.T) {
	f := newStillFixture(t)
	raw := stillJPEG(t)
	_, cache, root := stillFileCache(t, f)
	l := rawStillClaim(t, f)
	image, err := cache.Publish(f.ctx, l, raw)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	f.exec(t, `UPDATE media_stills SET size_bytes=0 WHERE item_id=$1`, f.itemID)
	if err := os.Remove(filepath.Join(root, image.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	next := rawStillClaim(t, f)
	image, err = cache.Publish(f.ctx, next, raw)
	if err != nil {
		t.Fatal("repair killed independent attempt", err)
	}
	if token := f.text(t, `SELECT lease_token::text FROM media_stills WHERE item_id=$1`, f.itemID); token != next.Token {
		t.Fatal("repair replaced lease")
	}
	if ok, err := f.store.Succeed(f.ctx, next, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || ok {
		t.Fatal("old worker wrote back", ok, err)
	}
	current, err := f.store.Current(f.ctx, f.itemID)
	if err != nil || current == nil {
		t.Fatal(err)
	}
	data, err := cache.Read(f.ctx, *current)
	if err != nil || !bytes.Equal(data, raw) {
		t.Fatal("new frame unreadable", err)
	}
}
func TestNativeStillStoreAdmissionDeletesOnlyCommittedVictimsAndBacksOff(t *testing.T) {
	f := newStillFixture(t)
	raw := stillJPEG(t)
	f.store = cacheRepository(t, f, int64(len(raw)*3))
	store, cache, root := stillFileCache(t, f)
	old := seedStillSnapshots(t, f, 3, int64(len(raw)))
	for i, s := range old {
		old[i] = writeSeedStill(t, f, store, s, raw)
	}
	l := rawStillClaim(t, f)
	image, err := cache.Publish(f.ctx, l, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, old[0].RelativePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("victim not removed", err)
	}
	if current, err := f.store.Current(f.ctx, old[0].ItemID); err != nil || current != nil {
		t.Fatal("deleted before clearing reference", err)
	}
	for _, s := range old[1:] {
		if _, err := os.Stat(filepath.Join(root, s.RelativePath)); err != nil {
			t.Fatal("wrong victim", err)
		}
	}
	if ok, err := f.store.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	// A denied publication gets a finite retry; the worker can feed this directly
	// into the existing independent state machine without using jobs retries.
	f.store = cacheRepository(t, f, int64(len(raw)-1))
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = stillstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cache, err = stillstore.NewCache(store, f.store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := f.store.Retry(f.ctx, f.itemID); err != nil {
		t.Fatal(err)
	}
	next := rawStillClaim(t, f)
	_, err = cache.Publish(f.ctx, next, raw)
	var retry *stillstore.RetryError
	if !errors.As(err, &retry) || retry.After <= 0 || retry.Reason != stillqueue.StorageFull {
		t.Fatal("not a finite retry", err)
	}
	if ok, err := f.store.Fail(f.ctx, next, stillqueue.Failure{Class: linksource.Retryable, Reason: string(retry.Reason), RetryAfter: retry.After}); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if f.state(t) != "pending" || f.integer(t, `SELECT count(*) FROM media_stills WHERE item_id=$1 AND next_attempt_at>clock_timestamp() AND next_attempt_at<=clock_timestamp()+interval '2 minutes'`, f.itemID) != 1 {
		t.Fatal("denial permanently blocked task")
	}
}
func TestNativeStillStoreGCSizeCalibrationQuotaAndFences(t *testing.T) {
	f := newStillFixture(t, 60)
	raw := stillJPEG(t)
	f.store = cacheRepository(t, f, int64(len(raw)*2))
	store, cache, root := stillFileCache(t, f)
	old := seedStillSnapshots(t, f, 3, 0)
	for i, s := range old {
		old[i] = writeSeedStill(t, f, store, s, raw)
	}
	r, err := cache.GC(f.ctx, 10)
	if err != nil || r.Visited > 10 {
		t.Fatal(r, err)
	}
	if u := cacheUsage(t, f.store); u.UnknownSizes != 0 || u.SnapshotBytes > f.store.Limits().MaxBytes*9/10 {
		t.Fatal("GC did not reconcile/trim quota", u)
	}
	for _, s := range old {
		current, err := f.store.Current(f.ctx, s.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(filepath.Join(root, s.RelativePath))
		if current == nil && !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("committed GC victim remains", statErr)
		}
		if current != nil && (statErr != nil || current.SizeBytes != int64(len(raw))) {
			t.Fatal("live reference lost", statErr)
		}
	}
	// A stale observation cannot change a newer snapshot's bookkeeping.
	current, err := f.store.Current(f.ctx, old[2].ItemID)
	if err != nil || current == nil {
		t.Fatal(err)
	}
	stale := *current
	stale.PublishedAt = stale.PublishedAt.Add(-time.Second)
	if ok, err := f.store.Calibrate(f.ctx, stale, 1); err != nil || ok {
		t.Fatal("stale calibration", ok, err)
	}
	for _, index := range []string{"media_stills_path", "media_stills_unknown_size"} {
		if f.integer(t, `SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, index) != 1 {
			t.Fatal(index)
		}
	}
	if _, err := f.provider.DownTo(f.ctx, 59); err != nil {
		t.Fatal(err)
	}
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"media_stills_path", "media_stills_unknown_size"} {
		if f.integer(t, `SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, index) != 1 {
			t.Fatal("roundtrip index", index)
		}
	}
}
func TestNativeStillStoreMaintenanceBoundsAndActiveReferences(t *testing.T) {
	f := newStillFixture(t)
	l := rawStillClaim(t, f)
	path, err := stillqueue.ImagePath(l.AssetID, l.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if used, err := f.store.Referenced(f.ctx, path); err != nil || !used {
		t.Fatal("active unpublished path unprotected", used, err)
	}
	if ok, err := f.store.Fail(f.ctx, l, stillqueue.Failure{Class: linksource.Retryable, Reason: string(stillqueue.StorageUnavailable)}); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if used, err := f.store.Referenced(f.ctx, path); err != nil || used {
		t.Fatal(used, err)
	}
	for _, limit := range []int{0, 201} {
		if _, err := f.store.SnapshotPage(f.ctx, 0, true, limit); err != stillqueue.ErrInvalid {
			t.Fatal(err)
		}
	}
	if _, err := f.store.Calibrate(f.ctx, stillqueue.Snapshot{}, -1); err != stillqueue.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := f.store.Referenced(f.ctx, ""); err != stillqueue.ErrInvalid {
		t.Fatal(err)
	}
	// Cancellation is passed through and never authorizes a filesystem action.
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.store.SnapshotPage(ctx, 0, false, 1); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestNativeStillStoreFailedRepairCommitKeepsFileAndCanRetry(t *testing.T) {
	f := newStillFixture(t)
	raw := stillJPEG(t)
	store, cache, root := stillFileCache(t, f)
	old := writeSeedStill(t, f, store, seedStillSnapshots(t, f, 1, 0)[0], raw)
	if err := os.WriteFile(filepath.Join(root, old.RelativePath), nil, 0600); err != nil {
		t.Fatal(err)
	} // Corrupt synthetic cache, not media.
	for _, ddl := range []string{
		`CREATE FUNCTION reject_still_repair() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic repair commit failure'; END;$$`,
		`CREATE CONSTRAINT TRIGGER reject_still_repair AFTER UPDATE ON media_stills DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW WHEN(OLD.published_at IS NOT NULL AND NEW.published_at IS NULL) EXECUTE FUNCTION reject_still_repair()`,
	} {
		if _, err := f.pool.Exec(f.ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	l := rawStillClaim(t, f)
	_, err := cache.Publish(f.ctx, l, raw)
	var retry *stillstore.RetryError
	if !errors.As(err, &retry) {
		t.Fatal("commit failure not retryable", err)
	}
	if current, err := f.store.Current(f.ctx, old.ItemID); err != nil || current == nil {
		t.Fatal("failed repair lost reference", err)
	}
	if _, err := os.Stat(filepath.Join(root, old.RelativePath)); err != nil {
		t.Fatal("uncommitted repair removed file", err)
	}
	if ok, err := f.store.Fail(f.ctx, l, stillqueue.Failure{Class: linksource.Retryable, Reason: string(retry.Reason), RetryAfter: retry.After}); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := f.pool.Exec(f.ctx, `DROP TRIGGER reject_still_repair ON media_stills`); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE media_stills SET next_attempt_at=clock_timestamp() WHERE item_id=$1`, f.itemID)
	next := rawStillClaim(t, f)
	image, err := cache.Publish(f.ctx, next, raw)
	if err != nil {
		t.Fatal("sticky repair refusal", err)
	}
	if ok, err := f.store.Succeed(f.ctx, next, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := os.Stat(filepath.Join(root, old.RelativePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("committed bad cache not removed", err)
	}
}
