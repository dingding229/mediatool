package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/nativelibrary"
)

type nativeCredit struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}
type nativeRecord struct {
	record
	VoteAverage  *float32 `json:"vote_average"`
	Runtime      int      `json:"runtime"`
	SeasonNumber *int     `json:"season_number"`
	Status       string   `json:"status"`
	LastAirDate  string   `json:"last_air_date"`
	ExternalIDs  struct {
		Imdb string `json:"imdb_id"`
	} `json:"external_ids"`
	ReleaseDates struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
	Credits struct {
		Cast []nativeCredit `json:"cast"`
		Crew []nativeCredit `json:"crew"`
	} `json:"credits"`
	Images struct {
		Posters, Backdrops, Logos []struct {
			Path string `json:"file_path"`
		}
	} `json:"images"`
	Episodes []struct {
		ID             int64 `json:"id"`
		Season         int   `json:"season_number"`
		Number         int   `json:"episode_number"`
		Name, Overview string
		AirDate        string `json:"air_date"`
		StillPath      string `json:"still_path"`
		Runtime        int    `json:"runtime"`
	} `json:"episodes"`
}

// NativeDetails uses the existing Token, language and explicitly scoped proxy.
// A season response contains episode metadata; no per-episode fan-out is needed.
func (c Client) NativeDetails(ctx context.Context, kind nativelibrary.TMDBDetailType, id int64, season *int, region string) (nativelibrary.TMDBDetails, error) {
	var out nativelibrary.TMDBDetails
	if id < 1 || (kind != nativelibrary.TMDBTypeMovie && kind != nativelibrary.TMDBTypeSeries && kind != nativelibrary.TMDBTypeSeason) || (kind == nativelibrary.TMDBTypeSeason && (season == nil || *season < 0 || *season > 999)) {
		return out, ErrRequest
	}
	prefix, appendices := "movie", "release_dates,external_ids,credits,images"
	if kind != nativelibrary.TMDBTypeMovie {
		prefix = "tv"
		appendices = "content_ratings,external_ids,credits,images"
	}
	endpoint := "/" + prefix + "/" + strconv.FormatInt(id, 10)
	if kind == nativelibrary.TMDBTypeSeason {
		endpoint += fmt.Sprintf("/season/%d", *season)
		appendices = "credits,images"
	}
	var r nativeRecord
	if err := c.getNativeRetry(ctx, endpoint, url.Values{"append_to_response": {appendices}, "include_image_language": {"zh,en,null"}}, &r); err != nil {
		return out, err
	}
	if kind != nativelibrary.TMDBTypeSeason && int64(r.ID) != id || kind == nativelibrary.TMDBTypeSeason && (r.SeasonNumber == nil || *r.SeasonNumber != *season || r.ID < 1) {
		return out, ErrRequest
	}
	out.Name, out.OriginalTitle, out.PremiereDate = r.Title, r.OriginalTitle, r.ReleaseDate
	if kind != nativelibrary.TMDBTypeMovie {
		out.Name, out.OriginalTitle, out.PremiereDate = r.Name, r.OriginalName, r.FirstAirDate
	}
	if out.PremiereDate != "" {
		if _, err := time.Parse("2006-01-02", out.PremiereDate); err != nil {
			return out, ErrRequest
		}
	}
	if len(out.Name) > 2000 || len(r.Overview) > 65536 || r.Runtime < 0 || r.Runtime > 1440 || r.VoteAverage != nil && (*r.VoteAverage < 0 || *r.VoteAverage > 10) {
		return out, ErrRequest
	}
	out.Overview, out.PosterPath, out.BackdropPath, out.CommunityRating, out.RuntimeMinutes, out.ImdbID = r.Overview, r.PosterPath, r.BackdropPath, r.VoteAverage, r.Runtime, r.ExternalIDs.Imdb
	out.EndDate, out.Status = r.LastAirDate, r.Status
	for _, g := range r.Genres {
		out.Genres = append(out.Genres, g.Name)
	}
	if region == "" {
		region = "CN"
	}
	for _, country := range []string{strings.ToUpper(region), "US"} {
		for _, result := range r.ReleaseDates.Results {
			if result.Country == country {
				for _, release := range result.Dates {
					if release.Certification != "" {
						out.OfficialRating = release.Certification
						break
					}
				}
			}
		}
		for _, result := range r.ContentRatings.Results {
			if result.Country == country && result.Rating != "" {
				out.OfficialRating = result.Rating
				break
			}
		}
		if out.OfficialRating != "" {
			break
		}
	}
	if out.PosterPath == "" && len(r.Images.Posters) > 0 {
		out.PosterPath = r.Images.Posters[0].Path
	}
	if out.BackdropPath == "" && len(r.Images.Backdrops) > 0 {
		out.BackdropPath = r.Images.Backdrops[0].Path
	}
	for _, image := range r.Images.Posters {
		if nativeImagePath(image.Path) {
			out.Posters = append(out.Posters, image.Path)
		}
	}
	for _, image := range r.Images.Backdrops {
		if nativeImagePath(image.Path) {
			out.Backdrops = append(out.Backdrops, image.Path)
		}
	}
	for _, image := range r.Images.Logos {
		if nativeImagePath(image.Path) {
			out.Logos = append(out.Logos, image.Path)
		}
	}
	for _, image := range r.Images.Posters {
		if nativeImagePath(image.Path) {
			out.Posters = append(out.Posters, image.Path)
		}
	}
	for _, image := range r.Images.Backdrops {
		if nativeImagePath(image.Path) {
			out.Backdrops = append(out.Backdrops, image.Path)
		}
	}
	for _, image := range r.Images.Logos {
		if nativeImagePath(image.Path) {
			out.Logos = append(out.Logos, image.Path)
		}
	}
	if len(r.Images.Logos) > 0 {
		out.LogoPath = r.Images.Logos[0].Path
	}
	for _, v := range r.Credits.Cast[:min(len(r.Credits.Cast), 20)] {
		out.People = append(out.People, nativelibrary.TMDBPerson{ID: v.ID, Name: v.Name, Type: "Actor", Role: v.Character, ImagePath: v.ProfilePath})
	}
	for _, v := range r.Credits.Crew {
		kind := ""
		if v.Job == "Director" {
			kind = "Director"
		} else if v.Department == "Writing" || v.Job == "Screenplay" || v.Job == "Writer" {
			kind = "Writer"
		}
		if kind != "" && len(out.People) < 60 {
			out.People = append(out.People, nativelibrary.TMDBPerson{ID: v.ID, Name: v.Name, Type: kind, Role: v.Job, ImagePath: v.ProfilePath})
		}
	}
	if len(r.Episodes) > 2000 {
		return out, ErrRequest
	}
	seen := map[int]bool{}
	for _, ep := range r.Episodes {
		if kind != nativelibrary.TMDBTypeSeason || ep.ID < 1 || ep.Season != *season || ep.Number < 1 || ep.Number > 9999 || seen[ep.Number] || ep.Runtime < 0 || ep.Runtime > 1440 || len(ep.Name) > 2000 || len(ep.Overview) > 65536 {
			return out, ErrRequest
		}
		seen[ep.Number] = true
		if ep.AirDate != "" {
			if _, err := time.Parse("2006-01-02", ep.AirDate); err != nil {
				return out, ErrRequest
			}
		}
		if !nativeImagePath(ep.StillPath) {
			return out, ErrRequest
		}
		out.Episodes = append(out.Episodes, nativelibrary.TMDBEpisode{ID: ep.ID, Season: ep.Season, Number: ep.Number, Name: ep.Name, Overview: ep.Overview, AirDate: ep.AirDate, StillPath: ep.StillPath, RuntimeMinutes: ep.Runtime})
	}
	for _, p := range []string{out.PosterPath, out.BackdropPath, out.LogoPath} {
		if !nativeImagePath(p) {
			return out, ErrRequest
		}
	}
	return out, nil
}
func nativeImagePath(value string) bool {
	return value == "" || (len(value) <= 2048 && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && path.Clean(value) == value && !strings.ContainsAny(value, "\\?#%\x00\r\n"))
}

func (c Client) getNativeRetry(ctx context.Context, endpoint string, query url.Values, out any) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := c.get(ctx, endpoint, query, out)
		if err == nil {
			return nil
		}
		var response *ResponseError
		if !errors.As(err, &response) || response.Status != 429 || attempt == 2 {
			return err
		}
		delay := response.RetryAfter
		if delay <= 0 {
			delay = time.Duration(1<<attempt) * time.Second
		}
		pauseRequests(delay)
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ErrRequest
		}
	}
	return ErrRequest
}
