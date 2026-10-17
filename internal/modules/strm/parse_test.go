package strm

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"media-tool/internal/modules/storage"
)

func TestParseStableSharesStrictGatewayPolicy(t *testing.T) {
	o := ParseOptions{Builder: Builder{BaseURL: "https://resolver.example", APIKey: "synthetic-key", AllowedRoots: []string{"/115"}}, Origins: []string{"https://alias.example/"}, LegacyEnabled: true}
	valid, err := o.Builder.Build(storage.FileRef{Path: "/115/synthetic.mkv", PickCode: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{valid, strings.Replace(valid, "resolver.example", "alias.example", 1)} {
		ref, err := ParseStable(context.Background(), raw, o)
		if err != nil || ref.Path != "/115/synthetic.mkv" || ref.PickCode != "abc123" {
			t.Fatal("valid legacy refused", err)
		}
	}
	invalid := []string{
		"%", valid + "#fragment", strings.Replace(valid, "resolver.example", "attacker.example", 1), strings.Replace(valid, "https://", "https://user@", 1), valid + "&apiKey=duplicate", valid + "&remoteUrl=https%3A%2F%2Fattacker.example", valid + "&unknown=1", valid + "\n", valid + "&bad=%zz", strings.Replace(valid, "synthetic-key", "wrong", 1), strings.Replace(valid, "/strm302", "/%73trm302", 1), strings.Replace(valid, "%2F115%2Fsynthetic.mkv", "%2Foutside%2Fsynthetic.mkv", 1), valid + strings.Repeat("x", 8192),
	}
	for i, raw := range invalid {
		if _, err := ParseStable(context.Background(), raw, o); err == nil {
			t.Fatalf("invalid %d accepted", i)
		}
	}
	o.LegacyEnabled = false
	if _, err := ParseStable(context.Background(), valid, o); err == nil {
		t.Fatal("legacy toggle ignored")
	}
	u, err := url.Parse(valid)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("assetId", "synthetic-id")
	u.RawQuery = q.Encode()
	managed := u.String()
	if _, err := ParseStable(context.Background(), managed, o); err == nil {
		t.Fatal("no grant verifier")
	}
	called := 0
	o.ManagedURL = func(ctx context.Context, raw string) (storage.FileRef, error) {
		called++
		if raw != managed {
			t.Error("body changed")
		}
		return storage.FileRef{AssetID: "synthetic-id", Path: "/115/synthetic.mkv"}, nil
	}
	ref, err := ParseStable(context.Background(), managed, o)
	if err != nil || ref.AssetID != "synthetic-id" || called != 1 {
		t.Fatal("managed verifier not used")
	}
	o.ManagedURL = func(context.Context, string) (storage.FileRef, error) {
		return storage.FileRef{}, errors.New("revoked")
	}
	if _, err := ParseStable(context.Background(), managed, o); err == nil {
		t.Fatal("grant failure ignored")
	}
}
