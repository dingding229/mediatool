package httpapi

import (
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/settings"
	"net/http"
)

func (a *API) getClassification(w http.ResponseWriter, r *http.Request) {
	out, err := (classification.Repository{DB: a.Deps.DB}).Get(r.Context())
	if err != nil {
		fail(w, 503, "classification_unavailable", err.Error())
		return
	}
	write(w, 200, out)
}
func (a *API) saveClassification(w http.ResponseWriter, r *http.Request) {
	var in classification.Record
	if !decodeLimit(w, r, &in, 200<<10) {
		return
	}
	out, err := (catalog.Repository{DB: a.Deps.DB}).SavePolicy(r.Context(), in.Value, in.Revision)
	if err != nil {
		fail(w, 409, "classification_invalid", err.Error())
		return
	}
	write(w, 200, out)
}
func (a *API) previewClassification(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Item     metadata.Item `json:"item"`
		Source   string        `json:"source"`
		Season   int           `json:"season"`
		Episodes []int         `json:"episodes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Item.ID < 1 || (in.Item.MediaType != "movie" && in.Item.MediaType != "series") {
		fail(w, 400, "invalid_media", "需要明确媒体身份")
		return
	}
	p, err := (classification.Repository{DB: a.Deps.DB}).Get(r.Context())
	if err != nil {
		fail(w, 503, "classification_unavailable", err.Error())
		return
	}
	s, err := (settings.Repository{DB: a.Deps.DB}).Get(r.Context())
	if err != nil {
		fail(w, 503, "settings_unavailable", err.Error())
		return
	}
	name, m, err := classification.Render(p.Value, in.Item, in.Source, s.Value.MovieTemplate, s.Value.SeriesTemplate, ".strm", in.Season, in.Episodes)
	if err != nil {
		fail(w, 400, "layout_invalid", err.Error())
		return
	}
	write(w, 200, map[string]any{"match": m, "path": name, "revision": p.Revision})
}
