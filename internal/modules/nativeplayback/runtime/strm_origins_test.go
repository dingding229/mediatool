package runtime

import (
	"context"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/config"
	"strings"
	"testing"
)

func TestSTRMOriginsAreIndependentOfHTTPSManagement(t *testing.T) {
	old := "http://192.0.2.10:8080"
	play := "https://play.example.test"
	key := "synthetic-key"
	raw := old + strm.Endpoint + "?path=%2F115%2Fsynthetic.mkv&apiKey=" + key
	base := config.Config{Environment: "production", PublicURL: "https://admin.example.test", STRMKey: key, STRMAllowedRoots: []string{"/115"}, LegacyPlaybackEnabled: true, STRMResolverOrigins: []string{old}}
	r := Resolver{Config: base}
	options := r.strmParseOptions(play)
	if _, err := strm.ParseStable(context.Background(), raw, options); err != nil {
		t.Fatal("explicit old source rejected", err)
	}
	r.Config.PublicURL = "https://another-admin.example.test"
	if _, err := strm.ParseStable(context.Background(), raw, r.strmParseOptions(play)); err != nil {
		t.Fatal("management change affected source", err)
	}
	for _, bad := range []string{strings.Replace(raw, old, "http://unknown.example.test", 1), strings.Replace(raw, old, base.PublicURL, 1), strings.Replace(raw, key, "wrong-signature", 1), strings.Replace(raw, "%2F115%2F", "%2Foutside%2F", 1)} {
		if _, err := strm.ParseStable(context.Background(), bad, options); err == nil {
			t.Fatal("unsafe source/key/path accepted")
		}
	}
	r.Config.LegacyPlaybackEnabled = false
	if _, err := strm.ParseStable(context.Background(), raw, r.strmParseOptions(play)); err == nil {
		t.Fatal("legacy toggle bypassed")
	}
	r.Config = base
	r.Config.Environment = "development"
	r.Config.PublicURL = old
	r.Config.STRMResolverOrigins = nil
	if _, err := strm.ParseStable(context.Background(), raw, r.strmParseOptions(play)); err != nil {
		t.Fatal("development fallback changed", err)
	}
	r.Config = base
	r.Config.STRMResolverOrigins = nil
	if _, err := strm.ParseStable(context.Background(), strings.Replace(raw, old, play, 1), r.strmParseOptions(play)); err != nil {
		t.Fatal("configured playback origin rejected", err)
	}
	if _, err := strm.ParseStable(context.Background(), raw, r.strmParseOptions(play)); err == nil {
		t.Fatal("unlisted old source accepted in production")
	}
}
