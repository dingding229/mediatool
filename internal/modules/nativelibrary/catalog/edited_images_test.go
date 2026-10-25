package catalog

import (
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/imageedit"
	"testing"
)

func TestEditedImageTagsRemainScopedAndInheritOnlyParents(t *testing.T) {
	parent := map[string]imageedit.Image{"Primary": {Tag: "parent-poster"}, "Backdrop": {Tag: "parent-bg"}, "Logo": {Tag: "parent-logo"}}
	movie := dto.BaseItemDto{Id: "100001", Type: dto.Movie}
	applyEditedImages(&movie, parent, nil)
	if movie.ImageTags["Primary"] != "parent-poster" || movie.SeriesPrimaryImageTag != "" || len(movie.ParentBackdropImageTags) != 0 {
		t.Fatal(movie)
	}
	episode := dto.BaseItemDto{Id: "100003", Type: dto.Episode, SeriesId: "100002"}
	applyEditedImages(&episode, map[string]imageedit.Image{"Primary": {Tag: "episode-thumb"}}, parent)
	if episode.ImageTags["Primary"] != "episode-thumb" || episode.SeriesPrimaryImageTag != "parent-poster" || episode.ParentLogoItemId != "100002" || episode.ParentBackdropItemId != "100002" {
		t.Fatal(episode)
	}
}
