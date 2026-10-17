package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/modules/metadata"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrRequest = errors.New("TMDB request failed; check access token or connectivity")

type Client struct {
	Token, Language string
	HTTP            *http.Client
	// BaseURL is test-only/controlled injection; production defaults to TMDB.
	BaseURL string
}

func (c Client) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	release, err := admitRequest(ctx)
	if err != nil {
		return err
	}
	defer release()
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	copyQuery := url.Values{}
	for key, values := range query {
		copyQuery[key] = append([]string(nil), values...)
	}
	language := c.Language
	if language == "" {
		language = "zh-CN"
	}
	copyQuery.Set("language", language)
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.themoviedb.org/3"
	}
	request, err := http.NewRequestWithContext(ctx, "GET", base+endpoint+"?"+copyQuery.Encode(), nil)
	if err != nil {
		return ErrRequest
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	response, err := client.Do(request)
	if err != nil {
		return ErrRequest
	}
	if response.StatusCode != http.StatusOK {
		delay := retryDelay(response.Header.Get("Retry-After"))
		if response.StatusCode == 429 {
			pauseRequests(max(delay, 2*time.Second))
		}
		if closeErr := response.Body.Close(); closeErr != nil {
			return ErrRequest
		}
		return &ResponseError{Status: response.StatusCode, RetryAfter: delay}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	closeErr := response.Body.Close()
	if err != nil || closeErr != nil || len(raw) > 4<<20 || json.Unmarshal(raw, out) != nil {
		return ErrRequest
	}
	return nil
}
func (c Client) Probe(ctx context.Context) error {
	var config map[string]any
	return c.get(ctx, "/configuration", nil, &config)
}

type record struct {
	OriginalLanguage    string   `json:"original_language"`
	OriginCountry       []string `json:"origin_country"`
	ProductionCountries []struct {
		Code string `json:"iso_3166_1"`
	} `json:"production_countries"`
	ID            int
	Title, Name   string
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	Overview      string
	PosterPath    string `json:"poster_path"`
	BackdropPath  string `json:"backdrop_path"`
	Genres        []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
	MediaType string `json:"media_type"`
}

func convert(r record, kind string) metadata.Item {
	title, original, date := r.Title, r.OriginalTitle, r.ReleaseDate
	if kind == "series" {
		title, original, date = r.Name, r.OriginalName, r.FirstAirDate
	}
	year := ""
	if len(date) >= 4 {
		year = date[:4]
	}
	genres := []string{}
	ids := []int{}
	countries := append([]string(nil), r.OriginCountry...)
	for _, c := range r.ProductionCountries {
		countries = append(countries, c.Code)
	}
	for _, g := range r.Genres {
		genres = append(genres, g.Name)
		ids = append(ids, g.ID)
	}
	return metadata.Item{GenreIDs: ids, Countries: countries, OriginalLanguage: r.OriginalLanguage, BackdropPath: r.BackdropPath, Genres: genres, ID: r.ID, MediaType: kind, Title: title, OriginalTitle: original, Year: year, Overview: r.Overview, PosterPath: r.PosterPath}
}
func (c Client) Search(ctx context.Context, kind, query string) ([]metadata.Item, error) {
	if len(query) > 200 || strings.TrimSpace(query) == "" {
		return nil, errors.New("enter a media search term")
	}
	path := "/search/movie"
	if kind == "auto" || kind == "" {
		path = "/search/multi"
		kind = "auto"
	}
	if kind == "series" {
		path = "/search/tv"
	} else if kind != "movie" && kind != "auto" {
		return nil, errors.New("media type must be auto, movie or series")
	}
	var result struct{ Results []record }
	if err := c.get(ctx, path, url.Values{"query": {query}, "include_adult": {"false"}}, &result); err != nil {
		return nil, err
	}
	out := []metadata.Item{}
	for _, r := range result.Results {
		resolved := kind
		if kind == "auto" {
			switch r.MediaType {
			case "movie":
				resolved = "movie"
			case "tv":
				resolved = "series"
			default:
				continue
			}
		}
		out = append(out, convert(r, resolved))
	}
	return out, nil
}
func (c Client) Details(ctx context.Context, kind string, id int) (metadata.Item, error) {
	if id <= 0 {
		return metadata.Item{}, errors.New("invalid TMDB id")
	}
	prefix := "movie"
	if kind == "series" {
		prefix = "tv"
	} else if kind != "movie" {
		return metadata.Item{}, errors.New("invalid media type")
	}
	var r record
	if err := c.get(ctx, "/"+prefix+"/"+strconv.Itoa(id), nil, &r); err != nil {
		return metadata.Item{}, err
	}
	return convert(r, kind), nil
}

type SeasonEpisode struct {
	Number  int    `json:"episode_number"`
	AirDate string `json:"air_date"`
}

func (c Client) SeasonEpisodes(ctx context.Context, id, season int) ([]int, error) {
	if id < 1 || season < 0 || season > 999 {
		return nil, errors.New("invalid series or season")
	}
	var result struct {
		Episodes []SeasonEpisode `json:"episodes"`
	}
	if err := c.get(ctx, "/tv/"+strconv.Itoa(id)+"/season/"+strconv.Itoa(season), nil, &result); err != nil {
		return nil, err
	}
	zone, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		zone = time.FixedZone("Asia/Shanghai", 8*3600)
	}
	today := time.Now().In(zone).Format("2006-01-02")
	episodes := []int{}
	for _, episode := range result.Episodes {
		if episode.Number > 0 && episode.AirDate != "" && episode.AirDate <= today {
			episodes = append(episodes, episode.Number)
		}
	}
	return episodes, nil
}
