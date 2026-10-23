package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"net/http"
	"strings"
	"time"
)

type fileIdentificationInput struct {
	Name      string `json:"name"`
	Query     string `json:"query"`
	MediaType string `json:"mediaType"`
	TMDBID    int    `json:"tmdbId"`
}

func (a *API) identifyFile(w http.ResponseWriter, r *http.Request) {
	var in fileIdentificationInput
	if !decode(w, r, &in) {
		return
	}
	if in.Name == "" || len(in.Name) > 2048 || len(in.Query) > 200 || in.TMDBID < 0 || (in.MediaType != "" && in.MediaType != "auto" && in.MediaType != "movie" && in.MediaType != "series") || in.TMDBID > 0 && in.MediaType != "movie" && in.MediaType != "series" {
		fail(w, 400, "invalid_file_identity", "请提供文件名和有效的 TMDB 媒体身份")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	policy, err := (classification.Repository{DB: a.Deps.DB}).Get(ctx)
	if err != nil {
		fail(w, 503, "classification_unavailable", "分类设置暂不可用，请重试")
		return
	}
	out := library.Recognition{Hint: releasename.Parse(in.Name)}
	engine := a.library()
	warning := ""
	if engine != nil {
		if in.TMDBID == 0 && strings.TrimSpace(in.Query) == "" {
			out, err = engine.RecognizeName(ctx, in.Name)
		} else {
			out.Hint = engine.NameHint(ctx, in.Name)
		}
	} else {
		err = errors.New("recognition unavailable")
	}
	// Search and choosing a different title are explicit user operations. Details
	// (not the partial search result) supply country/language/genre classification.
	if engine != nil && (in.TMDBID > 0 || strings.TrimSpace(in.Query) != "") {
		provider, e := a.Deps.Integrations.TMDB(ctx)
		err = e
		out.Selected = nil
		if err == nil && in.TMDBID > 0 {
			var item metadata.Item
			item, err = provider.Details(ctx, in.MediaType, in.TMDBID)
			if err == nil && item.ID == in.TMDBID && item.MediaType == in.MediaType {
				out.Selected = &item
			}
		}
		if err == nil && in.TMDBID == 0 {
			kind := in.MediaType
			if kind == "" {
				kind = "auto"
			}
			out.Matches, err = provider.Search(ctx, kind, strings.TrimSpace(in.Query))
		}
	} else if err == nil && out.Selected != nil {
		selected := out.Selected
		out.Selected = nil
		provider, e := a.Deps.Integrations.TMDB(ctx)
		err = e
		if err == nil {
			var item metadata.Item
			item, err = provider.Details(ctx, selected.MediaType, selected.ID)
			if err == nil && item.ID == selected.ID && metadata.UniqueExact([]metadata.Item{item}, out.Hint.Title, out.Hint.Year, out.Hint.MediaType) != nil {
				out.Selected = &item
			}
		}
	}
	if err != nil || out.Selected == nil {
		warning = "未识别，分类需手动选择"
	}
	if out.Matches == nil {
		out.Matches = []metadata.Item{}
	}
	var match *classification.Match
	if out.Selected != nil {
		m := policy.Value.Match(*out.Selected)
		match = &m
	}
	write(w, 200, map[string]any{"hint": out.Hint, "matches": out.Matches, "selected": out.Selected, "classification": match, "categories": map[string]any{"movie": policy.Value.Choices("movie"), "series": policy.Value.Choices("series")}, "warning": warning})
}
