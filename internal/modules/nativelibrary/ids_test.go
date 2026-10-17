package nativelibrary

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEntityKeysCanonicalAndBounded(t *testing.T) {
	for _, tc := range []struct {
		key   Key
		valid bool
	}{
		{Key{EntityMovie, "tmdb:1"}, true}, {Key{EntitySeries, "tmdb:1"}, true},
		{Key{EntitySeason, "tmdb:1:s0"}, true}, {Key{EntityEpisode, "tmdb:1:s0e1"}, true},
		{Key{EntityLibrary, "r:movie:documentary"}, true}, {Key{EntityLibrary, "g:movie"}, true}, {Key{EntityLibrary, "f:movie"}, true},
		{Key{EntityMovie, "tmdb:01"}, false}, {Key{EntityMovie, "tmdb:0"}, false},
		{Key{EntityMovie, "tmdb:9223372036854775808"}, false}, {Key{EntityMovie, "tmdb:1:s1"}, false},
		{Key{EntitySeason, "tmdb:1:s01"}, false}, {Key{EntityEpisode, "tmdb:1:s1e0"}, false},
		{Key{EntityEpisode, "tmdb:1:s1e2147483648"}, false}, {Key{EntityEpisode, "tmdb:1:s2147483648e1"}, false},
		{Key{EntityLibrary, "r:movie:../x"}, false}, {Key{EntityLibrary, "f:other"}, false},
		{Key{EntityLibrary, "g:" + strings.Repeat("x", 81)}, false}, {Key{"other", "tmdb:1"}, false},
	} {
		if got := tc.key.Valid(); got != tc.valid {
			t.Errorf("%+v Valid=%v", tc.key, got)
		}
	}
	for _, tc := range []struct {
		make func() (Key, error)
		want string
	}{
		{func() (Key, error) { return MovieKey(12) }, "tmdb:12"},
		{func() (Key, error) { return SeriesKey(12) }, "tmdb:12"},
		{func() (Key, error) { return SeasonKey(12, 0) }, "tmdb:12:s0"},
		{func() (Key, error) { return EpisodeKey(12, 1, 2) }, "tmdb:12:s1e2"},
	} {
		k, err := tc.make()
		if err != nil || k.InternalKey != tc.want {
			t.Fatal(k, err)
		}
	}
	for _, makeKey := range []func() (Key, error){func() (Key, error) { return MovieKey(0) }, func() (Key, error) { return SeriesKey(-1) }, func() (Key, error) { return SeasonKey(12, -1) }, func() (Key, error) { return EpisodeKey(12, 1, 0) }} {
		if _, err := makeKey(); !errors.Is(err, ErrInvalidKey) {
			t.Fatal("invalid constructor accepted", err)
		}
	}
}

type idTestRows struct {
	values            []func(...any) error
	index             int
	scanErr, finalErr error
	closed            bool
}

func (r *idTestRows) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}
func (r *idTestRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return r.values[r.index-1](dest...)
}
func (r *idTestRows) Err() error { return r.finalErr }
func (r *idTestRows) Close()     { r.closed = true }

type idTestQuery struct {
	calls int
	run   func(int, string, ...any) (IDRows, error)
}

func (q *idTestQuery) Query(_ context.Context, sql string, args ...any) (IDRows, error) {
	q.calls++
	return q.run(q.calls, sql, args...)
}
func idRow(dest ...any) error { *dest[0].(*int64) = 100001; return nil }
func keyRow(dest ...any) error {
	*dest[0].(*EntityType) = EntityMovie
	*dest[1].(*string) = "tmdb:12"
	*dest[2].(*int64) = 100001
	return nil
}

func TestEnsureIDsBatchesDeduplicatesAndClosesBothQueries(t *testing.T) {
	key := Key{EntityMovie, "tmdb:12"}
	insert, read := &idTestRows{values: []func(...any) error{idRow}}, &idTestRows{values: []func(...any) error{keyRow}}
	q := &idTestQuery{run: func(call int, sql string, args ...any) (IDRows, error) {
		var keys []Key
		if err := json.Unmarshal([]byte(args[0].(string)), &keys); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(keys, []Key{key}) {
			t.Fatal(keys)
		}
		if call == 1 {
			if !strings.Contains(sql, "ON CONFLICT") || !strings.Contains(sql, "DO NOTHING") {
				t.Fatal(sql)
			}
			return insert, nil
		}
		if !strings.Contains(sql, "retired_at=NULL") {
			t.Fatal(sql)
		}
		return read, nil
	}}
	got, err := EnsureIDs(context.Background(), q, []Key{key, key})
	if err != nil || got[key] != 100001 || q.calls != 2 || !insert.closed || !read.closed {
		t.Fatal(got, err, q.calls, insert.closed, read.closed)
	}
	if got, err := EnsureIDs(context.Background(), nil, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	for _, keys := range [][]Key{{{EntityMovie, "bad"}}, make([]Key, 501), {key}} {
		if _, err := EnsureIDs(context.Background(), nil, keys); !errors.Is(err, ErrInvalidKey) {
			t.Fatal(err)
		}
	}
}
func TestEnsureIDsPropagatesAllQueryAndRowFailures(t *testing.T) {
	failure := errors.New("synthetic database error")
	for _, at := range []string{"insert-query", "insert-scan", "insert-rows", "read-query", "read-scan", "read-rows", "incomplete"} {
		t.Run(at, func(t *testing.T) {
			insert, read := &idTestRows{values: []func(...any) error{idRow}}, &idTestRows{values: []func(...any) error{keyRow}}
			if at == "insert-scan" {
				insert.scanErr = failure
			}
			if at == "insert-rows" {
				insert.finalErr = failure
			}
			if at == "read-scan" {
				read.scanErr = failure
			}
			if at == "read-rows" {
				read.finalErr = failure
			}
			if at == "incomplete" {
				read.values = nil
			}
			q := &idTestQuery{run: func(n int, _ string, _ ...any) (IDRows, error) {
				if n == 1 {
					if at == "insert-query" {
						return nil, failure
					}
					return insert, nil
				}
				if at == "read-query" {
					return nil, failure
				}
				return read, nil
			}}
			got, err := EnsureIDs(context.Background(), q, []Key{{EntityMovie, "tmdb:12"}})
			if err == nil || got != nil {
				t.Fatal(got, err)
			}
			if at != "incomplete" && !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if at != "insert-query" && !insert.closed {
				t.Fatal("insert rows leaked")
			}
			if q.calls == 2 && at != "read-query" && !read.closed {
				t.Fatal("read rows leaked")
			}
		})
	}
}
