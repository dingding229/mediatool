package migrations_test

import (
	"fmt"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativepermissions"
	"media-tool/internal/modules/nativelibrary"
	"slices"
	"testing"
	"time"
)

func TestI10RemoveParentalRatingUpDownUpPreservesUsers(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 90); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO media_users(id,username,display_name,password_hash,enabled,must_change_password,credential_version,max_streams,max_parental_rating,expires_at)VALUES('10101010-0000-4000-8000-000000000091','i10.member','I10','synthetic-hash',true,true,7,3,65,'2030-01-01T00:00:00Z'),('20202020-0000-4000-8000-000000000091','i10.other','Other','synthetic-hash-2',false,false,9,2,NULL,NULL)`)
	snapshot := func() string {
		return f.text(t, `SELECT jsonb_agg(to_jsonb(u)-'max_parental_rating' ORDER BY id)::text FROM media_users u`)
	}
	before := snapshot()
	for _, version := range []int64{91, 90, 91} {
		var err error
		if version == 90 {
			_, err = f.provider.DownTo(f.ctx, version)
		} else {
			_, err = f.provider.UpTo(f.ctx, version)
		}
		if err != nil {
			t.Fatal(err)
		}
		f.version(t, version)
		t.Logf("schema=%d userCount=%d; all non-rating user fields compared", version, f.integer(t, `SELECT count(*) FROM media_users`))
		if snapshot() != before {
			t.Fatal("non-rating user fields changed")
		}
		want := int64(0)
		if version == 90 {
			want = 1
		}
		if f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='media_users' AND column_name='max_parental_rating'`) != want {
			t.Fatal("column shape")
		}
		if version == 90 {
			if f.integer(t, `SELECT count(*) FROM media_users WHERE max_parental_rating IS NOT NULL`) != 0 {
				t.Fatal("Down must restore structure only")
			}
			f.rejects(t, "23514", `UPDATE media_users SET max_parental_rating=-1`)
		}
	}
}

func TestI10EveryRatingVisibleToAuthorizedUsers(t *testing.T) {
	f := newBrowseFixture(t)
	ratings := []string{"G", "PG-13", "R", "TV-MA", "", "unknown"}
	for i, rating := range ratings {
		m := f.media(t, 9100000+int64(i), fmt.Sprintf("I10 Rated %d", i), "2026", fmt.Sprintf(`{"officialRating":%q}`, rating))
		f.asset(t, m, "g:test-a", time.Now(), "mkv")
	}
	for _, name := range []string{"i10.first", "i10.second"} {
		account, err := (mediausers.Repository{DB: f.pool}).Save(f.ctx, "", mediausers.Input{Username: name, Password: "Synthetic123", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		f.exec(t, `INSERT INTO media_user_library_grants(user_id,library_id,can_play)VALUES($1,'g:test-a',true)`, account.ID)
		user := contracts.User{ID: account.ID, Enabled: true, CredentialVersion: account.Revision, MaxStreams: 2}
		query := nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 2, SortBy: []string{"SortName"}}
		var seen []string
		for _, offset := range []int64{0, 2, 4} {
			query.StartIndex = offset
			items, total, err := f.repo.Query(f.ctx, user, nativepermissions.New(f.pool), query)
			if err != nil || total != 6 || len(items) != 2 {
				t.Fatal("all rating pagination", items, total, err)
			}
			for _, item := range items {
				seen = append(seen, item.OfficialRating)
			}
		}
		hints, err := f.repo.Discover(f.ctx, user, nativepermissions.New(f.pool), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{SearchTerm: "I10 Rated", IncludeItemTypes: []string{"Movie"}, Limit: 200}})
		if err != nil || hints.Hints.TotalRecordCount != 6 {
			t.Fatal("rated discovery hidden", hints, err)
		}
		slices.Sort(seen)
		want := slices.Clone(ratings)
		slices.Sort(want)
		if !slices.Equal(seen, want) {
			t.Fatal("rating metadata or visibility lost", seen)
		}
		f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1`, account.ID)
		query.StartIndex = 0
		items, total, err := f.repo.Query(f.ctx, user, nativepermissions.New(f.pool), query)
		if err != nil || total != 0 || len(items) != 0 {
			t.Fatal("library grant bypass", total, err)
		}
	}
}
