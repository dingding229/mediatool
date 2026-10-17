package migrations_test

import (
	"strings"
	"time"

	"media-tool/internal/modules/nativetmdb"
	"testing"
)

func TestNativeTMDBDetailsConstraintsRecoveryAndDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 58); err != nil {
		t.Fatal(err)
	}
	id := "aaaaaaaa-0000-4000-8000-000000000058"
	f.exec(t, `INSERT INTO native_tmdb_details(id,tmdb_type,tmdb_id,language)VALUES($1,'movie',900058,'zh-CN')`, id)
	f.exec(t, `UPDATE native_tmdb_details SET state='fetching',fetching_started_at=now()-interval '2 hours' WHERE id=$1`, id)
	f.exec(t, `UPDATE native_tmdb_details SET state='failed',fetching_started_at=NULL,next_attempt_at=now(),error='timeout' WHERE id=$1`, id)
	f.rejects(t, "23514", `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language)VALUES('movie',1,1,'zh-CN')`)
	f.rejects(t, "23514", `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,state)VALUES('season',2,'zh-CN','fetching')`)
	f.rejects(t, "23514", `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,state)VALUES('movie',3,'zh-CN','ready')`)
	f.rejects(t, "23505", `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language)VALUES('movie',900058,'zh-CN')`)
	large := strings.Repeat("x", 4<<20)
	f.rejects(t, "23514", `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload)VALUES('movie',900059,'zh-CN',jsonb_build_object('x',$1::text))`, large)
	if _, err := f.provider.Down(f.ctx); err != nil {
		t.Fatal("down 58", err)
	}
	if f.integer(t, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='native_tmdb_details'`) != 0 {
		t.Fatal("down left table")
	}
	if _, err := f.provider.UpTo(f.ctx, 58); err != nil {
		t.Fatal("up 58", err)
	}
	if f.integer(t, `SELECT count(*) FROM native_tmdb_details`) != 0 {
		t.Fatal("re-up did not recreate clean table")
	}
}

func TestNativeTMDBQueueMergesRefreshAndRecoversFetching(t *testing.T) {
	f := newMovieFixture(t)
	mid := f.media(t, 960001, "Synthetic Queue Film", "2026", `{}`)
	asset := f.asset(t, mid, "g:test-a", time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), "mkv")
	repo := nativetmdb.Repository{DB: f.pool}
	if err := repo.EnqueueAsset(f.ctx, asset.asset, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	id := f.text(t, `SELECT id::text FROM native_tmdb_details WHERE tmdb_type='movie' AND tmdb_id=960001`)
	if id == "" || f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.tmdb.fetch'`) != 1 {
		t.Fatal("initial enqueue missing")
	}
	if err := repo.EnqueueAsset(f.ctx, asset.asset, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.tmdb.fetch'`) != 1 {
		t.Fatal("duplicate TMDB jobs accumulated")
	}
	f.exec(t, `UPDATE native_tmdb_details SET state='fetching',fetching_started_at=now()-interval '2 hours' WHERE id=$1`, id)
	if _, _, err := repo.ScheduleBatch(f.ctx, "zh-CN", ""); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT state FROM native_tmdb_details WHERE id=$1`, id) != "failed" {
		t.Fatal("stale fetching did not converge")
	}
	key := "native.tmdb:movie:960001:-1:zh-CN"
	if f.integer(t, `SELECT count(*) FROM jobs WHERE idempotency_key=$1`, key) != 1 {
		t.Fatal("refresh queue key changed")
	}
	f.exec(t, `UPDATE jobs SET state='running',lease_until=now()+interval '1 minute',lease_token=gen_random_uuid(),locked_by='tmdb-test' WHERE idempotency_key=$1`, key)
	if _, err := repo.Enqueue(f.ctx, nativetmdb.Target{Type: "movie", TMDBID: 960001}, "zh-CN", true); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT payload->>'rerun' FROM jobs WHERE idempotency_key=$1`, key) != "true" {
		t.Fatal("running refresh did not merge rerun")
	}
}
