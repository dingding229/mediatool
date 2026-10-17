package migrations_test

import (
	"fmt"
	"testing"
	"time"

	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func TestNativeFallbackVisibilitySurvivesPlanAndStartupSync(t *testing.T) {
	for mask := 0; mask < 4; mask++ {
		t.Run(fmt.Sprintf("assigned_types_%d", mask), func(t *testing.T) {
			f := newNativeFixture(t)
			if _, err := f.provider.UpTo(f.ctx, 57); err != nil {
				t.Fatal(err)
			}
			pool := catalogTestPool(t, f)
			f.exec(t, `UPDATE directory_classification SET value='{"groups":[]}',revision=7 WHERE singleton`)
			f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('library','g:removed')`)
			f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type)SELECT 'g:removed',id,'removed','Removed synthetic group','Removed','movies' FROM native_item_ids WHERE entity_type='library' AND internal_key='g:removed'`)
			storage := jobs.UUID()
			f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'Synthetic fallback storage','clouddrive2')`, storage)
			for i, kind := range []string{"movie", "series"} {
				if mask&(1<<i) == 0 {
					continue
				}
				media, asset, file := jobs.UUID(), jobs.UUID(), jobs.UUID()
				f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,$2,$3,'Synthetic fallback','{}')`, media, kind, 330001+i)
				f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, file, storage, "/115/synthetic/"+file+".mkv")
				f.exec(t, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
				f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state,season,episode)VALUES($1,$2,'ready',1,1)`, asset, media)
				f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'synthetic-only')`, jobs.UUID(), asset, "Unclassified/"+asset+".strm")
				if err := (catalog.AssignmentRepository{DB: pool}).Assign(f.ctx, asset, 1); err != nil {
					t.Fatal(err)
				}
				if got := f.text(t, `SELECT library_id FROM native_asset_libraries WHERE asset_id=$1`, asset); got != "f:"+kind {
					t.Fatal("unexpected assignment", got)
				}
			}
			ids := f.text(t, `SELECT jsonb_object_agg(id,item_id::text)::text FROM native_libraries`)
			jobsBefore := f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.assign'`)
			// Cover both a previously hidden populated fallback and a wrongly
			// visible empty one. Sync must recompute from assignments, not plans.
			f.exec(t, `UPDATE native_libraries l SET hidden=EXISTS(SELECT 1 FROM native_asset_libraries a WHERE a.library_id=l.id) WHERE l.id IN('f:movie','f:series')`)
			check := func() {
				t.Helper()
				for i, kind := range []string{"movie", "series"} {
					want := int64(0)
					if mask&(1<<i) != 0 {
						want = 1
					}
					if got := f.integer(t, `SELECT count(*) FROM native_libraries WHERE id=$1 AND NOT hidden`, "f:"+kind); got != want {
						t.Fatalf("%s visibility=%d want=%d", kind, got, want)
					}
				}
				if f.integer(t, `SELECT count(*) FROM native_libraries WHERE id='g:removed' AND hidden`) != 1 {
					t.Fatal("removed ordinary library was not hidden")
				}
				if f.text(t, `SELECT jsonb_object_agg(id,item_id::text)::text FROM native_libraries`) != ids || f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.assign'`) != jobsBefore {
					t.Fatal("sync changed IDs or enqueued assignment jobs")
				}
			}
			if err := (catalog.Repository{DB: pool}).Sync(f.ctx, classification.Policy{Groups: []classification.Group{}}, 7, nativelibrary.GranularityRule); err != nil {
				t.Fatal(err)
			}
			check()
			// New pools/repositories exercise the exact SyncCurrent startup path
			// against persisted state, without relying on prior process memory.
			for restart := 0; restart < 3; restart++ {
				fresh := catalogTestPool(t, f)
				if err := (catalog.Repository{DB: fresh}).SyncCurrent(f.ctx); err != nil {
					fresh.Close()
					t.Fatal(err)
				}
				fresh.Close()
				check()
			}
		})
	}
}

func TestNativeFallbackSyncBrowseHierarchyAndNextUp(t *testing.T) {
	f := newBrowseFixture(t)
	f.exec(t, `UPDATE directory_classification SET value='{"groups":[]}',revision=7 WHERE singleton`)
	for _, lib := range []string{"f:movie", "f:series"} {
		f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,$2,true)`, f.user.ID, lib)
	}
	movie := f.media(t, 330011, "Synthetic fallback movie", "2026", `{"officialRating":"G"}`)
	f.asset(t, movie, "f:movie", time.Now(), "mkv")
	show := f.media(t, 330012, "Synthetic fallback series", "2026", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, show)
	f.episode(t, show, "f:series", 1, 1, 1)
	f.episode(t, show, "f:series", 1, 2, 2)
	if _, err := catalog.PreallocateIDBatch(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	// Reproduce production: existing assignments survive, but the previous
	// startup had hidden both fallback libraries.
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id IN('f:movie','f:series')`)
	if err := (catalog.Repository{DB: f.pool}).SyncCurrent(f.ctx); err != nil {
		t.Fatal(err)
	}
	views, err := f.repo.Views(f.ctx, f.user, f.permissions())
	if err != nil || len(views) != 2 {
		t.Fatal("fallback Views after startup", views, err)
	}
	movieParent := f.text(t, `SELECT item_id::text FROM native_libraries WHERE id='f:movie'`)
	seriesParent := f.text(t, `SELECT item_id::text FROM native_libraries WHERE id='f:series'`)
	movies, total := f.query(t, nativelibrary.ItemQuery{ParentID: movieParent, Limit: 200})
	if total != 1 || len(movies) != 1 || movies[0].ParentId != movieParent {
		t.Fatal("fallback movie browse", movies, total)
	}
	series, total := f.query(t, nativelibrary.ItemQuery{ParentID: seriesParent, Limit: 200})
	if total != 1 || len(series) != 1 {
		t.Fatal("fallback series browse", series, total)
	}
	hierarchy := catalog.HierarchyRepository{DB: f.pool}
	visible := nativelibrary.VisibleLibraries{RequestedID: "f:series", AllowedIDs: []string{"f:series"}}
	shows, count, err := hierarchy.ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: visible, Limit: 200})
	if err != nil || count != 1 || len(shows) != 1 || shows[0].Id != series[0].Id {
		t.Fatal("fallback hierarchy series", shows, count, err)
	}
	seasons, err := hierarchy.ListSeasons(f.ctx, visible, shows[0].Id)
	if err != nil || len(seasons) != 1 {
		t.Fatal("fallback hierarchy seasons", seasons, err)
	}
	episodes, count, err := hierarchy.ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: visible, SeriesID: shows[0].Id, SeasonID: seasons[0].Id, Limit: 200})
	if err != nil || count != 2 || len(episodes) != 2 {
		t.Fatal("fallback hierarchy episodes", episodes, count, err)
	}
	played, stamp := true, time.Now().UTC()
	if _, err := f.repo.PersonalData(f.ctx, f.user, f.permissions(), episodes[0].Id, &nativelibrary.PersonalPatch{Played: &played, LastPlayedAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	next, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.NextUp, ItemID: shows[0].Id, Items: nativelibrary.ItemQuery{Limit: 20}})
	if err != nil || next.Total != 1 || len(next.Items) != 1 || next.Items[0].Id != episodes[1].Id {
		t.Fatal("fallback NextUp", next, err)
	}
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1 AND library_id='f:series'`, f.user.ID)
	next, err = f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.NextUp, Items: nativelibrary.ItemQuery{Limit: 20}})
	if err != nil || next.Total != 0 || len(next.Items) != 0 {
		t.Fatal("fallback NextUp bypassed grants", next, err)
	}
	views, err = f.repo.Views(f.ctx, f.user, f.permissions())
	if err != nil || len(views) != 1 || views[0].Id != movieParent {
		t.Fatal("fallback Views bypassed grants", views, err)
	}
}
