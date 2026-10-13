package subscriptionpolicy

import (
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
	"testing"
)

func TestStructuredRulesAreSharedStrictAndExplainable(t *testing.T) {
	item := metadata.Item{Title: "Owned Film", OriginalTitle: "Owned Film", Year: "2026", MediaType: "movie"}
	r := search.Resource{Title: "Owned.Film.2026.4K.WEB.DL.x265-TEAM", Seeders: 2, Size: 1 << 30, Metadata: map[string]any{"hr": false}}
	rules := Rules{Resolution: "2160p", Source: "web-dl", Codec: "h265"}
	matched, reasons, checks := Evaluate(r, item, rules)
	if !matched || len(reasons) != 0 || len(checks) != 3 {
		t.Fatal(matched, reasons, checks)
	}
	for _, c := range checks {
		if c.State != "matched" {
			t.Fatal(c)
		}
	}
	rules.Resolution = "1080p"
	matched, reasons, checks = Evaluate(r, item, rules)
	if matched || len(reasons) != 1 || checks[0].State != "rejected" {
		t.Fatal(reasons, checks)
	}
	r.Title = "Owned Film 2026"
	_, reasons, checks = Evaluate(r, item, rules)
	if len(reasons) != 3 {
		t.Fatal(reasons)
	}
	for _, c := range checks {
		if c.Actual != "" || c.State != "rejected" {
			t.Fatal(c)
		}
	}
	matched, reasons = Filter(r, item, Rules{})
	if !matched || len(reasons) != 0 {
		t.Fatal("old rule behavior changed", reasons)
	}
}
func TestRejectHRRuleAppliesToKnownFlagsAndUnknownAlwaysFailsClosed(t *testing.T) {
	item := metadata.Item{Title: "Owned Film", Year: "2026", MediaType: "movie"}
	r := search.Resource{Title: "Owned.Film.2026", Metadata: map[string]any{"hr": true}}
	yes, no := true, false
	if ok, _ := Filter(r, item, Rules{RejectHR: &yes}); ok {
		t.Fatal("HR accepted")
	}
	if ok, _ := Filter(r, item, Rules{RejectHR: &no}); !ok {
		t.Fatal("explicitly allowed known HR rejected")
	}
	r.Metadata = map[string]any{}
	if ok, _ := Filter(r, item, Rules{RejectHR: &no}); ok {
		t.Fatal("unknown HR accepted")
	}
}
