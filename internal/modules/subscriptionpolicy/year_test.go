package subscriptionpolicy

import (
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
	"strings"
	"testing"
)

func TestAutomaticSeriesSubscriptionRejectsExplicitDifferentYear(t *testing.T) {
	media := metadata.Item{ID: 1, MediaType: "series", Title: "The Office", OriginalTitle: "The Office", Year: "2005"}
	for _, title := range []string{"The.Office.2019.S01E01.1080p.WEB-DL", "The.Office.S02E01.2019"} {
		r := search.Resource{Title: title, Metadata: map[string]any{"hr": false}}
		matched, reasons, _ := Evaluate(r, media, Rules{})
		if matched || !strings.Contains(strings.Join(reasons, ";"), "年份不一致") {
			t.Fatal(title, matched, reasons)
		}
	}
	for _, title := range []string{"The.Office.2005.S01E01.1080p.WEB-DL", "The.Office.S01E01.1080p.WEB-DL"} {
		if !TitleMatches(title, media) {
			t.Fatal("nonconflicting series rejected", title)
		}
	}
}
