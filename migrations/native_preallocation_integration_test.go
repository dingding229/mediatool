package migrations_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func TestNativeAssignmentPreallocatesWholeFamilyAndPeriodicRepair(t *testing.T) {
	f := newBrowseFixture(t)
	movie := f.media(t, 985001, "Synthetic preallocation", "2026", `{}`)
	asset := f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	process := func(id string) error {
		t.Helper()
		raw, err := json.Marshal(map[string]string{"assetId": id})
		if err != nil {
			t.Fatal(err)
		}
		return (catalog.Engine{DB: f.pool}).Process(f.ctx, jobs.Job{Payload: raw})
	}
	f.exec(t, `UPDATE strm_entries SET output_path='test-a/Movie.strm' WHERE asset_id=$1`, asset.asset)
	if err := process(asset.asset); err != nil {
		t.Fatal(err)
	}
	movieID := f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:985001'`)
	if err := process(asset.asset); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:985001'`) != movieID {
		t.Fatal("assignment replay changed ID")
	}
	f.library(t, "g:tv-prealloc", "tvshows", true, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',985002,'Synthetic Show','{}')`, show)
	ep := f.episode(t, show, "g:tv-prealloc", 0, 1, 2)
	f.exec(t, `UPDATE strm_entries SET output_path='tv-prealloc/Show.strm' WHERE asset_id=$1`, ep.asset)
	if err := process(ep.asset); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE internal_key IN('tmdb:985002','tmdb:985002:s0','tmdb:985002:s0e1') AND entity_type IN('series','season','episode') AND retired_at IS NULL`) != 3 {
		t.Fatal("incomplete episode ID family")
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE internal_key='tmdb:985002:s0e2'`) != 0 {
		t.Fatal("multi-file trailing episode allocated")
	}
	f.exec(t, `UPDATE native_item_ids SET retired_at=now() WHERE internal_key='tmdb:985002:s0e1'`)
	if err := process(ep.asset); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE internal_key='tmdb:985002:s0e1' AND retired_at IS NOT NULL`) != 0 {
		t.Fatal("reappearing ID not restored")
	}
	legacy := f.media(t, 985003, "Synthetic pre-existing assignment", "2026", `{}`)
	f.asset(t, legacy, "g:test-b", time.Now(), "mkv")
	n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
	if err != nil || n != 1 {
		t.Fatal("periodic missing-key repair", n, err)
	}
	n, err = catalog.PreallocateIDBatch(f.ctx, f.pool)
	if err != nil || n != 0 {
		t.Fatal("periodic repair not idempotent", n, err)
	}
	bad := f.media(t, 0, "Synthetic invalid identity", "2026", `{}`)
	badAsset := f.asset(t, bad, "g:test-a", time.Now(), "mkv")
	f.exec(t, `DELETE FROM native_asset_libraries WHERE asset_id=$1`, badAsset.asset)
	f.exec(t, `UPDATE strm_entries SET output_path='test-a/Bad.strm' WHERE asset_id=$1`, badAsset.asset)
	if err := process(badAsset.asset); err == nil {
		t.Fatal("failed ID allocation reported assignment success")
	}
	if f.integer(t, `SELECT count(*) FROM native_asset_libraries WHERE asset_id=$1`, badAsset.asset) != 0 {
		t.Fatal("assignment did not roll back on failed ID allocation")
	}
}
func TestNativeBrowseMissingIDsOnlyRepairsCurrentPageAndWarnsOnce(t *testing.T) {
	f := newBrowseFixture(t)
	for i := 0; i < 205; i++ {
		m := f.media(t, 986000+int64(i), fmt.Sprintf("Synthetic %03d", i), "2026", `{}`)
		f.asset(t, m, "g:test-a", time.Now(), "mkv")
	}
	var logs bytes.Buffer
	repo := catalog.NewBrowseRepository(f.pool, slog.New(slog.NewTextHandler(&logs, nil)))
	q := nativelibrary.ItemQuery{Limit: 200, Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}}
	items, total, err := repo.Query(f.ctx, f.user, f.permissions(), q)
	if err != nil || total != 205 || len(items) != 200 {
		t.Fatal(total, len(items), err)
	}
	if f.integer(t, `SELECT count(*) FROM native_item_ids WHERE entity_type='movie'`) != 200 {
		t.Fatal("fallback allocated outside page")
	}
	if strings.Count(logs.String(), "Native browse ID preallocation missing") != 1 {
		t.Fatal("missing preallocation warning")
	}
	writes := f.trace.idWrites.Load()
	if _, _, err := repo.Query(f.ctx, f.user, f.permissions(), q); err != nil {
		t.Fatal(err)
	}
	if f.trace.idWrites.Load() != writes {
		t.Fatal("normal repeated browse wrote IDs")
	}
	q.StartIndex = 200
	if _, _, err := repo.Query(f.ctx, f.user, f.permissions(), q); err != nil {
		t.Fatal(err)
	}
	if strings.Count(logs.String(), "Native browse ID preallocation missing") != 1 {
		t.Fatal("repeated warning")
	}

	for _, secret := range []string{"Browse123", "Token", "/115/"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("warning leaked sensitive content")
		}
	}
}
