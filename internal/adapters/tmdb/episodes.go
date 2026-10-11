package tmdb

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/metadata"
)

// EpisodeDetails fetches one season request and validates exactly the selected
// coverage. It does not expand sparse sets, filter future air dates, make up IDs
// or silently substitute a show's overview for a missing episode response.
func (c Client) EpisodeDetails(ctx context.Context, series, season int, numbers []int) ([]metadata.Episode, error) {
	if series < 1 || season < 0 || season > 999 || len(numbers) == 0 || len(numbers) > 100 {
		return nil, errors.New("explicit valid series/season/episode coverage required")
	}
	ordered := append([]int(nil), numbers...)
	sort.Ints(ordered)
	seen := map[int]bool{}
	for _, n := range ordered {
		if n < 1 || n > 9999 || seen[n] {
			return nil, errors.New("invalid or duplicate episode")
		}
		seen[n] = true
	}
	type episodeRecord struct {
		ID             int  `json:"id"`
		Number         int  `json:"episode_number"`
		Season         *int `json:"season_number"`
		Series         *int `json:"show_id"`
		Name, Overview string
		AirDate        string `json:"air_date"`
		Runtime        *int
		StillPath      string `json:"still_path"`
	}
	var reply struct {
		Season   *int `json:"season_number"`
		Episodes []episodeRecord
	}
	if err := c.get(ctx, "/tv/"+strconv.Itoa(series)+"/season/"+strconv.Itoa(season), nil, &reply); err != nil {
		return nil, err
	}
	if reply.Season == nil || *reply.Season != season || len(reply.Episodes) > 2000 {
		return nil, ErrRequest
	}
	selected := map[int]metadata.Episode{}
	for _, r := range reply.Episodes {
		if !seen[r.Number] {
			continue
		}
		if _, duplicate := selected[r.Number]; duplicate || r.ID < 1 || r.Season == nil || *r.Season != season || (r.Series != nil && *r.Series != series) || strings.TrimSpace(r.Name) == "" || len(r.Name) > 2000 || len(r.Overview) > 65536 {
			return nil, ErrRequest
		}
		if r.AirDate != "" {
			if _, err := time.Parse("2006-01-02", r.AirDate); err != nil {
				return nil, ErrRequest
			}
		}
		runtime := 0
		if r.Runtime != nil {
			runtime = *r.Runtime
		}
		if runtime < 0 || runtime > 1440 {
			return nil, ErrRequest
		}
		if r.StillPath != "" && (!strings.HasPrefix(r.StillPath, "/") || strings.ContainsAny(r.StillPath, "\\\r\n\x00") || len(r.StillPath) > 4096) {
			return nil, ErrRequest
		}
		selected[r.Number] = metadata.Episode{ID: r.ID, SeriesID: series, Season: season, Number: r.Number, Title: r.Name, Overview: r.Overview, AirDate: r.AirDate, Runtime: runtime, StillPath: r.StillPath}
	}
	out := []metadata.Episode{}
	for _, n := range ordered {
		e, ok := selected[n]
		if !ok {
			return nil, errors.New("selected episode absent from verified provider season; no output was staged")
		}
		out = append(out, e)
	}
	return out, nil
}

func (c Client) WithEpisodes(ctx context.Context, item metadata.Item, season int, numbers []int) (metadata.Item, error) {
	if item.MediaType != "series" {
		return item, nil
	}
	episodes, err := c.EpisodeDetails(ctx, item.ID, season, numbers)
	if err != nil {
		return item, err
	}
	item.EpisodeDetails = episodes
	return item, nil
}
