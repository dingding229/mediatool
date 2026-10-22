package metadata

import "media-tool/internal/platform/textkey"

// UniqueExact requires the same title (or original title), year and media kind.
// Ambiguous results, even duplicate provider rows, never authorize auto-import.
func UniqueExact(items []Item, title, year, kind string) *Item {
	if year == "" || textkey.Title(title) == "" || kind != "movie" && kind != "series" {
		return nil
	}
	var selected *Item
	for _, item := range items {
		if item.Year == year && item.MediaType == kind && (textkey.Title(item.Title) == textkey.Title(title) || textkey.Title(item.OriginalTitle) == textkey.Title(title)) {
			if selected != nil {
				return nil
			}
			copy := item
			selected = &copy
		}
	}
	return selected
}
