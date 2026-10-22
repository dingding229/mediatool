package nativelibrary

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"media-tool/internal/modules/embyapi/dto"
)

var tmdbImagePath = regexp.MustCompile(`^/[A-Za-z0-9_-]{1,200}\.(jpg|jpeg|png|webp)$`)

// ImagePath accepts only a single TMDB image filename, never a URL or a path
// traversal. TMDB supplies PNG renderings of SVG logos at the same basename.
func ImagePath(path string) string {
	if strings.HasSuffix(path, ".svg") {
		path = strings.TrimSuffix(path, ".svg") + ".png"
	}
	if !tmdbImagePath.MatchString(path) {
		return ""
	}
	return path
}

// ImagePaths gives the preferred image index zero, removes duplicates and uses
// exactly the same bounded order for tags and image responses.
func ImagePaths(preferred string, paths []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range append([]string{preferred}, paths...) {
		p = ImagePath(p)
		if p != "" && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
		if len(out) == 100 {
			break
		}
	}
	return out
}

// Person IDs occupy a fixed 32-hex namespace, disjoint from decimal item IDs.
// They are stable opaque protocol IDs, not authorization capabilities.
func PersonID(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("706572736f6e0000%016x", id)
}
func ParsePersonID(id string) (int64, bool) {
	if len(id) != 32 || !strings.HasPrefix(id, "706572736f6e0000") {
		return 0, false
	}
	n, err := strconv.ParseInt(id[16:], 16, 64)
	return n, err == nil && n > 0 && PersonID(n) == id
}
func applyImageTags(item *dto.BaseItemDto, poster, backdrop string, d *TMDBDetails) {
	if d == nil {
		d = &TMDBDetails{}
	}
	if item.ImageTags == nil {
		item.ImageTags = map[string]string{}
	}
	if p := ImagePath(poster); p != "" {
		item.ImageTags["Primary"] = ImageTag(p)
	}
	if logos := ImagePaths(d.LogoPath, d.Logos); len(logos) > 0 {
		item.ImageTags["Logo"] = ImageTag(logos[0])
	}
	item.BackdropImageTags = dto.Array[string]{}
	for _, p := range ImagePaths(backdrop, d.Backdrops) {
		item.BackdropImageTags = append(item.BackdropImageTags, ImageTag(p))
	}
}
func applyParentImageTags(item *dto.BaseItemDto, seriesID string, d *TMDBDetails) {
	if d == nil {
		return
	}
	if p := ImagePath(d.PosterPath); p != "" {
		item.SeriesPrimaryImageTag = ImageTag(p)
	}
	for _, p := range ImagePaths(d.BackdropPath, d.Backdrops) {
		item.ParentBackdropImageTags = append(item.ParentBackdropImageTags, ImageTag(p))
	}
	if len(item.ParentBackdropImageTags) > 0 {
		item.ParentBackdropItemId = seriesID
	}
	if logos := ImagePaths(d.LogoPath, d.Logos); len(logos) > 0 {
		item.ParentLogoItemId = seriesID
		item.ParentLogoImageTag = ImageTag(logos[0])
	}
}
