package catalog

import (
	"context"
	"errors"
	"media-tool/internal/modules/nativelibrary"
	"testing"
)

func TestMovieRepositoryRejectsInvalidBoundsBeforeDatabaseAccess(t *testing.T) {
	for _, q := range []MovieQuery{{LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 201}, {LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 0}, {LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 1, StartIndex: -1}, {LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 1, StartIndex: 1000001}, {LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 1, SortBy: "unsafe SQL"}, {Limit: 1}} {
		if _, _, err := (MovieRepository{}).Query(context.Background(), q); !errors.Is(err, ErrInvalid) {
			t.Fatal(q, err)
		}
	}
	if _, _, err := (MovieRepository{}).Query(context.Background(), MovieQuery{LibraryID: "g:test", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test", AllowedIDs: []string{"g:test"}}, Limit: 200}); !errors.Is(err, nativelibrary.ErrSettingsUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := RefreshIDLifecycle(context.Background(), nil, 0); !errors.Is(err, nativelibrary.ErrInvalidKey) {
		t.Fatal(err)
	}
	if _, err := (IDQueries{}).Query(context.Background(), "synthetic"); !errors.Is(err, nativelibrary.ErrSettingsUnavailable) {
		t.Fatal(err)
	}
}
