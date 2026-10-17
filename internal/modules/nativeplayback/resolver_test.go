package nativeplayback

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"testing"
)

func TestEmptyResolverCannotProduceTargetOrRelay(t *testing.T) {
	for i := 0; i < 2; i++ {
		target, err := NewStreamResolver().Resolve(context.Background(), contracts.User{}, "100001", "source-1", "SyntheticPlayer/1", "203.0.113.7")
		if !errors.Is(err, ErrNotImplemented) || target.URL != "" || !target.ExpiresAt.IsZero() {
			t.Fatal(target, err)
		}
	}
	raw, err := json.Marshal(RedirectTarget{URL: "https://synthetic.invalid/private?token=synthetic"})
	if err != nil || string(raw) != "{}" {
		t.Fatal("internal target serialized", string(raw), err)
	}
}
