package migrations_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"media-tool/internal/modules/nativelibrary"
)

func TestB5MoviePageMatchesGenericProjection(t *testing.T) {
	f := newBrowseFixture(t)
	for i := range 8 {
		m := f.media(t, 892100+int64(i), fmt.Sprintf("Page %02d", i), fmt.Sprint(2024+i%2), fmt.Sprintf(`{"genres":["Drama"],"sortName":"Same","communityRating":%d,"officialRating":"G","premiereDate":"2025-01-01"}`, i))
		f.asset(t, m, "g:test-a", time.Now(), "mkv")
		if i%2 == 0 {
			f.asset(t, m, "g:test-b", time.Now(), "mp4")
		}
	}
	initial, _ := f.query(t, nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie"}, Limit: 200})
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,played,favorite,position_ticks,play_count)VALUES($1,$2,true,true,1234,2)`, f.user.ID, initial[0].Id)
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES('movie',892101,'zh-CN','{"sortName":"Remote","genres":["Action"],"officialRating":"R","communityRating":9}','ready',now())`)
	for _, q := range []nativelibrary.ItemQuery{
		{SortBy: []string{"SortName"}}, {SortBy: []string{"CommunityRating"}, SortOrder: []string{"Descending"}},
		{SortBy: []string{"DatePlayed"}}, {Years: []string{"2024"}}, {Genres: []string{"action"}},
		{IsFavorite: boolPtr(true)}, {IsPlayed: boolPtr(false)}, {IsFolder: boolPtr(true)},
		{StartIndex: 3}, {ParentID: f.text(t, `SELECT item_id::text FROM native_libraries WHERE id='g:test-a'`)},
	} {
		q.Limit = 3
		q.Recursive = boolPtr(true)
		q.IncludeItemTypes = []string{"Movie"}
		fast, n := f.query(t, q)
		// Duplicating a type is semantically identical but bypasses the single-type
		// optimization, providing an independent generic-projection oracle.
		q.IncludeItemTypes = []string{"Movie", "Movie"}
		generic, total := f.query(t, q)
		if n != total || !reflect.DeepEqual(fast, generic) {
			t.Fatalf("optimized/generic drift: query=%+v count=%d/%d\nfast=%+v\ngeneric=%+v", q, n, total, fast, generic)
		}
	}
}
