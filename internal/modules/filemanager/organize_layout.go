package filemanager

import (
	"context"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"path"
	"strings"
)

func (e Engine) organizeLayout(ctx context.Context, in Input, source File) (Input, error) {
	hint := releasename.Parse(source.Name)
	if in.Title == "" {
		in.Title = hint.Title
	}
	if in.Year == "" {
		in.Year = hint.Year
	}
	if in.MediaType == "" || in.MediaType == "unknown" {
		in.MediaType = hint.MediaType
		in.Season, in.Episode = hint.Season, hint.Episode
	}
	if in.Episode == 0 && in.MediaType == "series" {
		in.Episode = hint.Episode
	}
	item := metadata.Item{ID: in.TMDBID, Title: in.Title, Year: in.Year, MediaType: in.MediaType}
	if in.TMDBID < 0 {
		return in, ErrInvalid
	}
	if in.TMDBID > 0 {
		provider, err := e.Connections.TMDB(ctx)
		if err != nil {
			return in, ErrUnavailable
		}
		item, err = provider.Details(ctx, in.MediaType, in.TMDBID)
		if err != nil {
			return in, ErrUnavailable
		}
		if item.ID != in.TMDBID || item.MediaType != in.MediaType {
			return in, ErrChanged
		}
	}
	if item.MediaType != "movie" && item.MediaType != "series" {
		return in, ErrInvalid
	}
	if item.MediaType == "movie" {
		in.Season, in.Episode = 0, 0
	}
	policy, err := (classification.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return in, ErrUnavailable
	}
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return in, ErrUnavailable
	}
	return organizeLayout(in, source, item, policy.Value, prefs.Value.MovieTemplate, prefs.Value.SeriesTemplate)
}

// Use the automatic ingestion renderer, including its sanitizing and season
// guards. Only the selected category or root may override its destination.
func organizeLayout(in Input, source File, item metadata.Item, policy classification.Policy, movieTemplate, seriesTemplate string) (Input, error) {
	if !source.Directory && in.Name == "" && len(releasename.Parse(source.Name).Episodes) > 1 {
		return in, ErrInvalid
	}
	match := policy.Match(item)
	category := in.Category
	if category == "" {
		category = match.Directory
	}
	allowed := false
	for _, choice := range policy.Choices(item.MediaType) {
		if choice.Directory == category {
			allowed = true
		}
	}
	if !allowed {
		return in, ErrInvalid
	}
	in.Title, in.Year, in.MediaType, in.TMDBID, in.Category = item.Title, item.Year, item.MediaType, item.ID, category
	if item.MediaType == "movie" {
		in.Season, in.Episode = 0, 0
	}
	in.media = &item
	if in.Name != "" && in.Template != "" {
		return in, ErrInvalid
	}
	base := in.Destination
	if _, err := Relative(base); err != nil {
		return in, err
	}
	if base == "" || base == "." {
		base = category
	}
	if source.Directory {
		if in.Template != "" {
			return in, ErrInvalid
		}
		if in.Name == "" {
			in.Name = source.Name
		}
		in.Destination = base
		return in, nil
	}
	if in.Name != "" && in.Destination != "" && in.Destination != "." {
		in.Destination = base
		return in, nil
	}
	if in.Template != "" {
		movieTemplate, seriesTemplate = in.Template, in.Template
	}
	episodes := []int{}
	if item.MediaType == "series" {
		episodes = []int{in.Episode}
	}
	hint := releasename.Parse(source.Name)
	if in.Resolution != "" {
		hint.Resolution = in.Resolution
	}
	rendered, _, err := classification.RenderWithHint(policy, item, source.Name, movieTemplate, seriesTemplate, path.Ext(source.Name), in.Season, episodes, hint)
	if err != nil {
		return in, ErrInvalid
	}
	relative := strings.TrimPrefix(rendered, match.Directory+"/")
	in.Destination = path.Join(base, path.Dir(relative))
	if in.Name == "" {
		in.Name = path.Base(relative)
	}
	in.Template = ""
	return in, nil
}
