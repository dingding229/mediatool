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
		{Limit: -1}, {StartIndex: -1}, {StartIndex: 1000001}, {IDs: make([]string, 201)},
		{Genres: make([]string, 65)}, {Years: make([]string, 101)},
		{SortBy: []string{"SortName"}, SortOrder: []string{"descending;drop"}},
		{SearchTerm: strings.Repeat("a", 257)}, {Years: []string{"20xx"}}, {Years: []string{"999"}}, {Genres: []string{strings.Repeat("a", 129)}}, {IDs: []string{""}},
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

func TestF2DomainQueryToleratesUnknownValuesWithoutSQLExpansion(t *testing.T) {
	for _, tc := range []struct{ sorts, orders, wantSorts, wantOrders []string }{
		{[]string{"SQL;DROP"}, nil, nil, nil},
		{[]string{"Name", "unknown", "DateCreated"}, []string{"Descending", "Ascending", "Descending"}, []string{"Name", "DateCreated"}, []string{"Descending", "Descending"}},
		{[]string{"Name", "DateCreated", "ProductionYear"}, []string{"Descending", "Descending"}, []string{"Name", "DateCreated", "ProductionYear"}, []string{"Descending", "Descending", "Ascending"}},
	} {
		q := nativelibrary.ItemQuery{Limit: 500, SortBy: tc.sorts, SortOrder: tc.orders, IncludeItemTypes: []string{"Person", "Playlist", "BoxSet"}, ExcludeItemTypes: []string{"Unknown"}}
		if e := validateBrowseQuery(&q); e != nil || q.Limit != 200 || strings.Join(q.SortBy, ",") != strings.Join(tc.wantSorts, ",") || strings.Join(q.SortOrder, ",") != strings.Join(tc.wantOrders, ",") {
			t.Fatalf("%+v %v", q, e)
		}
		w, args := conditions(q, browseScope{})
		if !strings.Contains(w, "kind=ANY($6::text[])") || strings.Join(args[5].([]string), ",") != "__unsupported__,__unsupported__,__unsupported__" {
			t.Fatalf("filter widened: %s %+v", w, args)
		}
	}
}

func TestF2UnknownWireTypesCannotAliasInternalKinds(t *testing.T) {
	for _, typ := range []string{"library", "person", "playlist", "boxset", "__unsupported__"} {
		if itemKind(typ) != "__unsupported__" {
			t.Fatal("unknown wire type mapped to a stored kind", typ)
		}
	}
	if itemKind("CollectionFolder") != "library" || itemKind("Movie") != "movie" {
		t.Fatal("known type not preserved")
	}
}
