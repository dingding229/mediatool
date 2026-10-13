package sites

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"regexp"
	"strings"
)

// Only an explicit compiled-template contract can classify a row without a
// discount marker as normal. Unrecognized future promotion markers stay unknown.
func promotionFactor(rule Map, row any, parsed *float64) *float64 {
	contract := object(rule["promotion"])
	if len(contract) == 0 {
		return parsed
	}
	node, ok := row.(*html.Node)
	if !ok {
		return parsed
	}
	matcher, err := cascadia.Compile(text(contract["marker_selector"]))
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, raw := range slice(contract["known_markers"]) {
		known[text(raw)] = true
	}
	nodes := goquery.NewDocumentFromNode(node).FindMatcher(matcher)
	unknown := false
	nodes.Each(func(_ int, s *goquery.Selection) {
		cl, _ := s.Attr("class")
		for _, token := range strings.Fields(cl) {
			if strings.HasPrefix(token, "pro_") || strings.HasPrefix(token, "torrent-pro-") {
				if !known[token] {
					unknown = true
				}
			}
		}
		alt, _ := s.Attr("alt")
		if strings.Contains(alt, "%") || strings.Contains(strings.ToLower(alt), "free") {
			if !regexp.MustCompile(`(?i)^(?:free|freeleech|0%|30%|50%|100%)$`).MatchString(strings.TrimSpace(alt)) {
				unknown = true
			}
		}
	})
	if unknown {
		return nil
	}
	if parsed != nil {
		return parsed
	}
	// Known discount-looking markers not handled by the declared numeric rule
	// cannot fall through to normal. Refund charging is explicitly normal (1).
	hasDiscount := false
	nodes.Each(func(_ int, s *goquery.Selection) {
		cl, _ := s.Attr("class")
		alt, _ := s.Attr("alt")
		value := strings.ToLower(cl + " " + alt)
		if strings.Contains(value, "free") || strings.Contains(value, "pctdown") || strings.Contains(value, "halfdown") || strings.Contains(value, "thirtypercent") || strings.Contains(value, "50%") || strings.Contains(value, "30%") {
			hasDiscount = true
		}
	})
	if hasDiscount {
		return nil
	}
	value, valid := checkedNumber(contract["normal_factor"])
	if !valid || value != 1 {
		return nil
	}
	return &value
}
