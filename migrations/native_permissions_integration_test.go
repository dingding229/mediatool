package migrations_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/mediausers/nativepermissions"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func TestNativePermissionsVisibleLibrariesComposeWithMovieAndHierarchy(t *testing.T) {
	f := newMovieFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE native_libraries SET collection_type='tvshows' WHERE id='g:test-b'`)
	movie := f.media(t, 990042, "Synthetic permission movie", "2026", `{}`)
	f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	series := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',990043,'Synthetic permission show','{}')`, series)
	episode := f.asset(t, series, "g:test-b", time.Now(), "mkv")
	f.exec(t, `UPDATE media_asset_bindings SET season=1,episode=1,episodes='[1]' WHERE asset_id=$1`, episode.asset)
	account, err := (mediausers.Repository{DB: f.pool}).Save(f.ctx, "", mediausers.Input{Username: "permission.member", Password: "Synthetic123", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	login, err := (&nativeauth.Store{DB: f.pool, Secret: "synthetic-permission-secret"}).Login(f.ctx, account.Username, "Synthetic123", contracts.DeviceInfo{DeviceID: "test-device"})
	if err != nil {
		t.Fatal(err)
	}
	u := login.User
	assertCatalog := func(ids []string, want int) {
		t.Helper()
		movies, n, err := (catalog.MovieRepository{DB: f.pool}).Query(f.ctx, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: ids}, Limit: 200})
		if len(movies) != want || n != want || want > 0 && err != nil {
			t.Fatal("movie visibility", len(movies), n, err)
		}
		shows, n, err := (catalog.HierarchyRepository{DB: f.pool}).ListSeries(f.ctx, nativelibrary.HierarchyQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-b", AllowedIDs: ids}, Limit: 200})
		if len(shows) != want || n != want || want > 0 && err != nil {
			t.Fatal("series visibility", len(shows), n, err)
		}
		if want > 0 {
			seasons, err := (catalog.HierarchyRepository{DB: f.pool}).ListSeasons(f.ctx, nativelibrary.VisibleLibraries{RequestedID: "g:test-b", AllowedIDs: ids}, shows[0].Id)
			if err != nil || len(seasons) != 1 {
				t.Fatal("season visibility", seasons, err)
			}
			episodes, n, err := (catalog.HierarchyRepository{DB: f.pool}).ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-b", AllowedIDs: ids}, SeriesID: shows[0].Id, Limit: 200})
			if err != nil || len(episodes) != 1 || n != 1 {
				t.Fatal("episode visibility", episodes, n, err)
			}
		}
	}
	c := nativepermissions.New(f.pool)
	ids, err := c.VisibleLibraries(f.ctx, u)
	if err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
	assertCatalog(ids, 0)
	for _, id := range []string{"g:test-a", "g:test-b"} {
		f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play,can_download)VALUES($1,$2,false,false)`, account.ID, id)
	}
	fresh := nativepermissions.New(f.pool)
	ids, err = fresh.VisibleLibraries(f.ctx, u)
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	assertCatalog(ids, 1)
	if err := fresh.CanBrowse(f.ctx, u, "g:test-a"); err != nil {
		t.Fatal(err)
	}
	if err := fresh.CanPlay(f.ctx, u, "g:test-a"); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("browse-only can play", err)
	}
	f.exec(t, `UPDATE media_user_library_grants SET can_play=true,can_download=true WHERE user_id=$1`, account.ID)
	newest := nativepermissions.New(f.pool)
	if err := newest.CanDownload(f.ctx, u, "g:test-a"); err != nil {
		t.Fatal("download grant", err)
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id IN('g:test-a','g:test-b')`)
	ids, err = nativepermissions.New(f.pool).VisibleLibraries(f.ctx, u)
	if err != nil || len(ids) != 0 {
		t.Fatal("hidden leaked", ids, err)
	}
	assertCatalog(ids, 0)
	if f.integer(t, `SELECT count(*) FROM media_user_library_grants WHERE user_id=$1`, account.ID) != 2 {
		t.Fatal("hiding deleted grants")
	}
	// The same request keeps its consistent snapshot. A new request sees changes.
	ids, err = fresh.VisibleLibraries(f.ctx, u)
	if err != nil || len(ids) != 2 {
		t.Fatal("request snapshot changed", ids, err)
	}
}

func TestNativePermissionsDefenseParentalAndStreamSlotBoundaries(t *testing.T) {
	f, store, account := authFixture(t)
	login, err := store.Login(f.ctx, account.Username, "Original-密码123", contracts.DeviceInfo{DeviceID: "slot-device"})
	if err != nil {
		t.Fatal(err)
	}
	u := login.User
	ctx := f.ctx
	// Use existing valid library item for the FK; this tests counters, not playback opening.
	ids, err := nativelibrary.EnsureIDs(ctx, catalog.IDQueries{DB: store.DB}, []nativelibrary.Key{{EntityType: nativelibrary.EntityLibrary, InternalKey: "g:slots"}})
	if err != nil {
		t.Fatal(err)
	}
	item := ids[nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: "g:slots"}]
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,name,directory,collection_type)VALUES('g:slots',$1,'slots','Slots','Slots','movies')`, item)
	f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'g:slots',true)`, account.ID)
	a, b := jobs.UUID(), jobs.UUID()
	for i, id := range []string{a, b, jobs.UUID(), jobs.UUID()} {
		state, progress := "playing", "1 minute"
		if i == 1 {
			state = "paused"
			progress = "4 minutes 59 seconds"
		}
		if i == 2 {
			progress = "5 minutes 1 second"
		}
		if i == 3 {
			state = "stopped"
		}
		f.exec(t, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state,started_at,last_progress_at,ended_at)VALUES($1,$2,$3,$4,'synthetic',$5,now()-interval '20 minutes',now()-$6::interval,CASE WHEN $5='stopped' THEN now() END)`, id, account.ID, login.Device.ID, item, state, progress)
	}
	slots, err := nativepermissions.New(store.DB).StreamSlots(ctx, u)
	if slots.InUse != 2 || slots.Limit != 2 || !errors.Is(err, contracts.ErrStreamLimit) {
		t.Fatal(slots, err)
	}
	f.exec(t, `UPDATE native_playback_sessions SET state='stopped',ended_at=now() WHERE id=$1`, a)
	slots, err = nativepermissions.New(store.DB).StreamSlots(ctx, u)
	if err != nil || slots.InUse != 1 {
		t.Fatal(slots, err)
	}
	f.exec(t, `UPDATE media_users SET max_streams=1,max_parental_rating=65 WHERE id=$1`, account.ID)
	slots, err = nativepermissions.New(store.DB).StreamSlots(ctx, u)
	if slots.Limit != 1 || !errors.Is(err, contracts.ErrStreamLimit) {
		t.Fatal("stale client limits trusted", slots, err)
	}
	for _, rating := range []string{"G", "PG-13", "R", "", "unknown"} {
		err := nativepermissions.New(store.DB).ItemAllowed(ctx, u, contracts.ItemAccess{LibraryIDs: []string{"g:other", "g:slots"}, OfficialRating: rating})
		if (err == nil) != (rating == "G" || rating == "PG-13") {
			t.Fatal("rating", rating, err)
		}
	}
	for _, update := range []string{"enabled=false", "enabled=true,expires_at=now()-interval '1 second'", "expires_at=NULL,credential_version=credential_version+1"} {
		f.exec(t, `UPDATE media_users SET `+update+` WHERE id=$1`, account.ID)
		if _, err := nativepermissions.New(store.DB).VisibleLibraries(ctx, u); !errors.Is(err, contracts.ErrForbidden) {
			t.Fatal("database account defense", strings.Split(update, "=")[0], err)
		}
	}
}
