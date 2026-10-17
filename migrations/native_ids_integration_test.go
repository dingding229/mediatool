package migrations_test

import (
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"strconv"
	"sync"
	"testing"
)

func TestNativeEnsureIDsConcurrentStableRetiredAndBatchBound(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 57); err != nil {
		t.Fatal(err)
	}
	pool := catalogTestPool(t, f)
	defer pool.Close()
	movie, err := nativelibrary.MovieKey(120001)
	if err != nil {
		t.Fatal(err)
	}
	series, err := nativelibrary.SeriesKey(120001)
	if err != nil {
		t.Fatal(err)
	}
	season, err := nativelibrary.SeasonKey(120001, 1)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := nativelibrary.EpisodeKey(120001, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(chan int64, 16)
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, []nativelibrary.Key{movie})
			if e != nil {
				errs <- e
				return
			}
			ids <- got[movie]
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		} else if id != first {
			t.Fatal("concurrent IDs differ", first, id)
		}
	}
	if first < 100000 {
		t.Fatal(first)
	}
	got, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, []nativelibrary.Key{movie, series, season, episode})
	if err != nil {
		t.Fatal(err)
	}
	if got[movie] != first || got[movie] == got[series] || got[season] == got[episode] {
		t.Fatal(got)
	}
	f.exec(t, `UPDATE native_item_ids SET retired_at=now() WHERE entity_type='movie' AND internal_key=$1`, movie.InternalKey)
	got, err = nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, []nativelibrary.Key{movie})
	if err != nil || got[movie] != first {
		t.Fatal(got, err)
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie' AND internal_key=$1 AND retired_at IS NULL`, movie.InternalKey) != 1 {
		t.Fatal("retired key not restored")
	}
	tooMany := make([]nativelibrary.Key, 501)
	for i := range tooMany {
		tooMany[i] = nativelibrary.Key{EntityType: nativelibrary.EntityMovie, InternalKey: "tmdb:" + strconv.Itoa(i+1)}
	}
	if _, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: pool}, tooMany); err == nil {
		t.Fatal("batch bound missing")
	}
}
