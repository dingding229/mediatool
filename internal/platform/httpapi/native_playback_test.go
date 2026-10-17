package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"strings"
	"testing"
)

func TestNativeGlobalSTRMKeyIsRejectedBeforeTokenStore(t *testing.T) {
	a := nativeAccounts{AccountService: &nativeauth.Store{}, global: strings.Repeat("a", 64)}
	if _, _, _, err := a.Authenticate(context.Background(), a.global, contracts.DeviceInfo{}); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, _, _, err := a.Authenticate(context.Background(), strings.Repeat("b", 64), contracts.DeviceInfo{}); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal("user credential not delegated", err)
	}
}
