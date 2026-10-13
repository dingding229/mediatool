package subscriptions

import (
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
	"testing"
)

func TestPreciseMediaRuleIdentityAndUnknownPromotion(t *testing.T) {
	item := metadata.Item{ID: 1, MediaType: "movie", Title: "幽灵公主", OriginalTitle: "Princess Mononoke", Year: "1997"}
	for _, title := range []string{"Princess.Mononoke.1997.1080p", "[Release] 幽灵公主 (1997)"} {
		if !TitleMatches(title, item) {
			t.Fatal(title)
		}
	}
	for _, title := range []string{"Princess Mononoke 2026", "Princess Mononoke Documentary 1997", "Princess Mononoke 1080p"} {
		if TitleMatches(title, item) {
			t.Fatal("ambiguous media accepted", title)
		}
	}
	resource := search.Resource{Title: "Princess.Mononoke.1997.1080p", Size: 100, Seeders: 2, Metadata: map[string]any{"hr": false}}
	rule := Rules{FreeOnly: true}
	if ok, _ := Filter(resource, item, rule); ok {
		t.Fatal("unknown promotion accepted")
	}
	zero := float64(0)
	resource.DownloadFactor = &zero
	if ok, _ := Filter(resource, item, rule); !ok {
		t.Fatal("known free resource rejected")
	}
	resource.Metadata = map[string]any{}
	if ok, _ := Filter(resource, item, rule); ok {
		t.Fatal("unknown HR accepted")
	}
	resource.Metadata = map[string]any{"hr": false}
	if ok, _ := Filter(resource, item, rule); !ok {
		t.Fatal("explicit HR-free rejected")
	}
	resource.Metadata["hr"] = true
	if ok, _ := Filter(resource, item, rule); ok {
		t.Fatal("HR resource downloaded")
	}
}
func TestManifestPackCoverageAndUnknownEpisode(t *testing.T) {
	episodes, err := ManifestCoverage([]string{"Show.S01E01.mkv", "Show.S01E02E03.mkv", "Sample/sample.mkv", "poster.jpg"}, 1)
	if err != nil || len(episodes) != 3 {
		t.Fatal(episodes, err)
	}
	if _, err = ManifestCoverage([]string{"Show.mkv"}, 1); err == nil {
		t.Fatal("guessed unknown episode")
	}
}

func TestStructuredSubscriptionInputRejectsUnknownAndSupportsOldRecords(t *testing.T) {
	in := Input{MediaType: "movie", TMDBID: 1, Source: "search", IntervalMinutes: 30, Rules: Rules{AccountIDs: []string{"owned"}}}
	if Validate(in) != nil {
		t.Fatal("old record changed")
	}
	in.Rules.Resolution = "2160p"
	in.Rules.Source = "web-dl"
	in.Rules.Codec = "h265"
	if Validate(in) != nil {
		t.Fatal("valid filters")
	}
	in.Rules.Resolution = "unknown"
	if Validate(in) == nil {
		t.Fatal("unknown is not a proved subscription attribute")
	}
}
