package pluginactor

import (
	"context"
	"errors"
	"testing"
)

func TestHostActorScopeAndSelectorCopy(t *testing.T) {
	if _, e := Require(context.Background(), "mediaserver.read"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	ids := []string{"g:movie"}
	ctx := With(context.Background(), "host-instance", "mediaserver.read", ids)
	ids[0] = "forged"
	a, e := Require(ctx, "mediaserver.read")
	if e != nil || !a.Allows("g:movie") || a.Allows("forged") {
		t.Fatal(a, e)
	}
	copied := a.Libraries()
	copied[0] = "forged"
	if !a.Allows("g:movie") {
		t.Fatal("mutable authority")
	}
	if _, e = Require(ctx, "mediaserver.users.write"); !errors.Is(e, ErrDenied) {
		t.Fatal("scope elevation", e)
	}
	if _, e = Require(With(ctx, "", "mediaserver.read", nil), "mediaserver.read"); !errors.Is(e, ErrDenied) {
		t.Fatal("empty actor", e)
	}
}
