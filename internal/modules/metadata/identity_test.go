package metadata

import "testing"

func TestUniqueExactIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, title, year, kind string
		items                   []Item
		want                    bool
	}{
		{"exact", "Owned Film", "2026", "movie", []Item{{ID: 1, Title: "Owned Film", Year: "2026", MediaType: "movie"}}, true},
		{"original", "Owned Film", "2026", "movie", []Item{{ID: 1, Title: "自有影片", OriginalTitle: "Owned Film", Year: "2026", MediaType: "movie"}}, true},
		{"ambiguous", "Owned Film", "2026", "movie", []Item{{ID: 1, Title: "Owned Film", Year: "2026", MediaType: "movie"}, {ID: 2, Title: "Owned Film", Year: "2026", MediaType: "movie"}}, false},
		{"no year", "Owned Film", "", "movie", []Item{{ID: 1, Title: "Owned Film", MediaType: "movie"}}, false},
		{"wrong kind", "Owned Film", "2026", "series", []Item{{ID: 1, Title: "Owned Film", Year: "2026", MediaType: "movie"}}, false},
		{"similar", "Owned Film", "2026", "movie", []Item{{ID: 1, Title: "Owned Film Again", Year: "2026", MediaType: "movie"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := UniqueExact(tc.items, tc.title, tc.year, tc.kind); (got != nil) != tc.want {
				t.Fatal(got)
			}
		})
	}
}
