package nativelibrary

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"testing"
)

func TestEmptyServicesAndProjectionStubsNeverClaimSuccess(t *testing.T) {
	ctx, user, query := context.Background(), contracts.User{}, ItemQuery{Limit: 100}
	for repeat := 0; repeat < 2; repeat++ {
		libraries := NewLibraryService()
		if rows, err := libraries.ListLibraries(ctx, user); !errors.Is(err, ErrNotImplemented) || rows == nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
		if item, err := libraries.GetLibrary(ctx, user, "100000"); !errors.Is(err, ErrNotImplemented) || item.ID != "" {
			t.Fatal(item, err)
		}
		items := NewItemService()
		if item, err := items.Get(ctx, user, "100001"); !errors.Is(err, ErrNotImplemented) || item.ID != "" {
			t.Fatal(item, err)
		}
		for name, call := range map[string]func() ([]Item, int, error){
			"Query":    func() ([]Item, int, error) { return items.Query(ctx, user, query) },
			"Seasons":  func() ([]Item, int, error) { return items.Seasons(ctx, user, "100002", query) },
			"Episodes": func() ([]Item, int, error) { return items.Episodes(ctx, user, "100002", query) },
			"Latest":   func() ([]Item, int, error) { return items.Latest(ctx, user, query) },
			"Search":   func() ([]Item, int, error) { return items.Search(ctx, user, "Synthetic", query) },
			"Resume":   func() ([]Item, int, error) { return NewUserDataService().Resume(ctx, user, query) },
			"NextUp":   func() ([]Item, int, error) { return NewUserDataService().NextUp(ctx, user, query) },
		} {
			t.Run(name, func(t *testing.T) {
				rows, total, err := call()
				if !errors.Is(err, ErrNotImplemented) || rows == nil || len(rows) != 0 || total != 0 {
					t.Fatal(rows, total, err)
				}
			})
		}
		data := NewUserDataService()
		for _, call := range []func() (UserData, error){
			func() (UserData, error) { return data.Get(ctx, user, "100001") },
			func() (UserData, error) { return data.Update(ctx, user, "100001", UserDataPatch{}) },
			func() (UserData, error) { return data.MarkPlayed(ctx, user, "100001", true) },
			func() (UserData, error) { return data.MarkFavorite(ctx, user, "100001", false) },
		} {
			if value, err := call(); !errors.Is(err, ErrNotImplemented) || value.Played || value.Favorite || value.PositionTicks != 0 {
				t.Fatal(value, err)
			}
		}
		if _, err := LibraryDTO(Library{}); !errors.Is(err, ErrInvalidProjection) {
			t.Fatal(err)
		}
		if _, err := ItemDTO(Item{}); !errors.Is(err, ErrNotImplemented) {
			t.Fatal(err)
		}
		if _, err := UserDataDTO(UserData{}); !errors.Is(err, ErrNotImplemented) {
			t.Fatal(err)
		}
	}
}
