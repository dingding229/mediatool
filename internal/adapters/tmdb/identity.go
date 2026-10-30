package tmdb

import (
	"context"
	"media-tool/internal/modules/metadata"
	"net/url"
	"strconv"
	"strings"
)

// IdentityDetails obtains aliases and external IDs in one provider request at
// selection time. Search filtering only reads the resulting frozen media JSON.
func (c Client) IdentityDetails(ctx context.Context, kind string, id int) (metadata.Item, error) {
	prefix := "movie"
	if kind == "series" {
		prefix = "tv"
	} else if kind != "movie" {
		return metadata.Item{}, ErrRequest
	}
	if id < 1 {
		return metadata.Item{}, ErrRequest
	}
	var r struct {
		record
		AlternativeTitles struct{ Titles, Results []struct{ Title string } } `json:"alternative_titles"`
		Translations      struct {
			Translations []struct{ Data struct{ Name, Title string } }
		} `json:"translations"`
		ExternalIDs struct {
			IMDBID string `json:"imdb_id"`
		} `json:"external_ids"`
	}
	if err := c.get(ctx, "/"+prefix+"/"+strconv.Itoa(id), url.Values{"append_to_response": {"alternative_titles,translations,external_ids"}}, &r); err != nil {
		return metadata.Item{}, err
	}
	if r.ID != id {
		return metadata.Item{}, ErrRequest
	}
	item := convert(r.record, kind)
	seen := map[string]bool{}
	add := func(title string) {
		title = strings.TrimSpace(title)
		if title != "" && len(title) <= 512 && len(item.CandidateTitles) < 256 && !seen[title] {
			seen[title] = true
			item.CandidateTitles = append(item.CandidateTitles, title)
		}
	}
	for _, v := range r.AlternativeTitles.Titles {
		add(v.Title)
	}
	for _, v := range r.AlternativeTitles.Results {
		add(v.Title)
	}
	for _, v := range r.Translations.Translations {
		add(v.Data.Name)
		add(v.Data.Title)
	}
	item.IMDBID = strings.TrimSpace(r.ExternalIDs.IMDBID)
	return item, nil
}
