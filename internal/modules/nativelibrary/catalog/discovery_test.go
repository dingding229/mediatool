package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

func TestDiscoveryRejectsInvalidRequestsAndGenreIDsAreStable(t *testing.T) {
	for _, q := range []nativelibrary.DiscoveryQuery{
		{Mode: nativelibrary.Hints}, {Mode: nativelibrary.Hints, Items: nativelibrary.ItemQuery{SearchTerm: "   ", Limit: 20}},
		{Mode: "injection;drop"}, {Mode: nativelibrary.Episodes, ItemID: strings.Repeat("1", 65)},
	} {
		_, err := (BrowseRepository{}).Discover(context.Background(), contracts.User{}, nil, q)
		if !errors.Is(err, nativelibrary.ErrInvalidQuery) {
			t.Fatal(q, err)
		}
	}
	one, two := GenreID("drama"), GenreID("drama")
	if one != two || len(one) != 70 || !strings.HasPrefix(one, "genre:") || one == GenreID("action") {
		t.Fatal("unstable/colliding genre identity")
	}
}
