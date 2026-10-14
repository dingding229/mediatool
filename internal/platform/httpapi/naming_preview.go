package httpapi

import (
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"net/http"
)

func (a *API) namingPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Movie  string `json:"movieTemplate"`
		Series string `json:"seriesTemplate"`
	}
	if !decode(w, r, &in) {
		return
	}
	pref := settings.Defaults()
	pref.MovieTemplate = in.Movie
	pref.SeriesTemplate = in.Series
	if settings.Validate(pref) != nil {
		fail(w, 400, "template_invalid", "相对路径模板无效：检查变量、目录分隔与季集标签")
		return
	}
	movie, err := releasename.RenderPath(in.Movie, releasename.Hint{Title: "示例电影", Year: "1997", MediaType: "movie", Resolution: "1080p", Extension: ".mkv"}, "Sample.1997.1080p.mkv")
	if err != nil {
		fail(w, 400, "template_invalid", err.Error())
		return
	}
	series, err := releasename.RenderPath(in.Series, releasename.Hint{Title: "示例剧集", Year: "2026", MediaType: "series", Season: 1, Episode: 4, Resolution: "1080p", Extension: ".mkv"}, "Sample.S01E04.1080p.mkv")
	if err != nil {
		fail(w, 400, "template_invalid", err.Error())
		return
	}
	write(w, 200, map[string]string{"movie": movie, "series": series})
}
