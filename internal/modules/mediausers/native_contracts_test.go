package mediausers

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"testing"
)

func TestNativeContractAliasesAreAvailableWithoutChangingExistingUser(t *testing.T) {
	if _, _, _, err := NewTokenAuthenticator().Authenticate(context.Background(), "synthetic", contracts.DeviceInfo{}); !errors.Is(err, contracts.ErrNotImplemented) {
		t.Fatal(err)
	}
	if err := NewPermissionChecker().CanBrowse(context.Background(), contracts.User{}, "g:synthetic"); !errors.Is(err, contracts.ErrNotImplemented) {
		t.Fatal(err)
	}
}
