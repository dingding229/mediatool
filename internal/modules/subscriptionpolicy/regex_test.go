package subscriptionpolicy

import (
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
	"testing"
)

func TestRegexKeywordPolicyFailsClosedAndPreservesLegacyLiteralSemantics(t *testing.T) {
	item := metadata.Item{MediaType: "movie", Title: "Owned Film", OriginalTitle: "Owned Film", Year: "2026"}
	r := search.Resource{Title: "Owned.Film.2026.1080p.WEB-DL.x265", Metadata: map[string]any{"hr": false}}
	rules := Rules{RegexKeywords: true, Include: []string{`(?i)web[-.]dl`, `x26[45]`}, Exclude: []string{`(?i)(cam|sample)`}}
	if ValidateKeywords(rules) != nil {
		t.Fatal("valid pattern rejected")
	}
	if ok, why := Filter(r, item, rules); !ok {
		t.Fatal(why)
	}
	rules.Include = []string{`WEB-DL`}
	r.Title = "Owned.Film.2026.web-dl.x265"
	if ok, _ := Filter(r, item, rules); ok {
		t.Fatal("regex silently case folded")
	}
	rules.Include = []string{`(?i)WEB-DL`}
	if ok, why := Filter(r, item, rules); !ok {
		t.Fatal(why)
	}
	rules.Exclude = []string{`[`}
	if ValidateKeywords(rules) == nil {
		t.Fatal("invalid pattern accepted")
	}
	if ok, _ := Filter(r, item, rules); ok {
		t.Fatal("invalid exclusion allowed resource")
	}
	rules.RegexKeywords = false
	rules.Exclude = nil
	rules.Include = []string{`web-dl`}
	if ok, why := Filter(r, item, rules); !ok {
		t.Fatal("old literal matching changed", why)
	}
}
