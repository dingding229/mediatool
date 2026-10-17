package contracts

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestEmptyNativeAuthenticationAndPermissionsFailClosed(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		user, device, token, err := NewTokenAuthenticator().Authenticate(ctx, "synthetic-user-token", DeviceInfo{DeviceID: "synthetic-device"})
		if !errors.Is(err, ErrNotImplemented) || !reflect.DeepEqual(user, User{}) || !reflect.DeepEqual(device, Device{}) || !reflect.DeepEqual(token, Token{}) {
			t.Fatal("empty auth returned identity", err)
		}
		permissions := NewPermissionChecker()
		for _, err := range []error{permissions.CanBrowse(ctx, user, "g:synthetic"), permissions.CanPlay(ctx, user, "g:synthetic"), permissions.CanDownload(ctx, user, "g:synthetic"), permissions.ParentalAllowed(ctx, user, ItemAccess{}), permissions.ItemAllowed(ctx, user, ItemAccess{})} {
			if !errors.Is(err, ErrNotImplemented) {
				t.Fatal("empty permission succeeded", err)
			}
		}
		if ids, err := permissions.VisibleLibraries(ctx, user); ids != nil || !errors.Is(err, ErrNotImplemented) {
			t.Fatal(ids, err)
		}
		if slots, err := permissions.StreamSlots(ctx, user); !errors.Is(err, ErrNotImplemented) || slots != (StreamCapacity{}) {
			t.Fatal(slots, err)
		}
	}
}
