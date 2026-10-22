package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

func TestBrowseRejectsUnboundedAndUnsafeInputBeforeDatabase(t *testing.T) {
	for _, q := range []nativelibrary.ItemQuery{
		{Limit: 201}, {StartIndex: -1}, {StartIndex: 1000001}, {IDs: make([]string, 201)},
		{Genres: make([]string, 65)}, {Years: make([]string, 101)}, {SortBy: []string{"name;drop"}},
		{SortBy: []string{"SortName"}, SortOrder: []string{"descending;drop"}},
		{SortBy: []string{"SortName"}, SortOrder: []string{"Ascending", "Descending"}},
		{SearchTerm: strings.Repeat("a", 257)}, {Years: []string{"20xx"}}, {Years: []string{"999"}}, {Genres: []string{strings.Repeat("a", 129)}}, {IDs: []string{""}}, {IncludeItemTypes: []string{"any"}},
	} {
		_, _, err := (BrowseRepository{}).Query(context.Background(), contracts.User{}, nil, q)
		if !errors.Is(err, nativelibrary.ErrInvalidQuery) {
			t.Fatal(q, err)
		}
	}
	q := nativelibrary.ItemQuery{SortBy: []string{"Random", "DateCreated"}, SortOrder: []string{"Ascending", "Descending"}}
	if err := validateBrowseQuery(&q); err != nil || len(q.SortBy) != 1 || q.SortBy[0] != "DateCreated" || q.SortOrder[0] != "Descending" {
		t.Fatal(q, err)
	}
	if _, err := (BrowseRepository{}).Views(context.Background(), contracts.User{}, nil); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := (BrowseRepository{}).Counts(context.Background(), contracts.User{}, nil); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestStandardUnstoredTypesRemainBoundFilters(t *testing.T) {
	q := nativelibrary.ItemQuery{IncludeItemTypes: []string{"Movie", "Video", "MusicVideo", "MusicAlbum"}, ExcludeItemTypes: []string{"Video"}, SortBy: []string{"DateLastContentAdded", "DateCreated", "SortName"}, SortOrder: []string{"Descending"}}
	if err := validateBrowseQuery(&q); err != nil {
		t.Fatal(err)
	}
	where, args := conditions(q, browseScope{})
	if !strings.Contains(where, "kind=ANY($6::text[])") || !strings.Contains(where, "NOT(kind=ANY($7::text[]))") || strings.Join(args[5].([]string), ",") != "movie,video,musicvideo,musicalbum" {
		t.Fatalf("filter dropped or expanded: %s %+v", where, args)
	}
	if browseSorts["DateLastContentAdded"] != "latest_created" {
		t.Fatal("last-content-added must use the newest physical member, not the original creation date")
	}
}
