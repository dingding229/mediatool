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
}

func (c Client) get(ctx context.Context, path string, query url.Values, out any) error {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	if query == nil {
		query = url.Values{}
	}
	language := c.Language
	if language == "" {
		language = "zh-CN"
	}
	query.Set("language", language)
	r, err := http.NewRequestWithContext(ctx, "GET", "https://api.themoviedb.org/3"+path+"?"+query.Encode(), nil)
	if err != nil {
		return ErrRequest
	}
	r.Header.Set("Authorization", "Bearer "+c.Token)
	response, err := client.Do(r)
	if err != nil {
		return ErrRequest
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ErrRequest
	}
	if json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(out) != nil {
		return ErrRequest
	}
	return nil
}
func (c Client) Probe(ctx context.Context) error {
	var config map[string]any
	return c.get(ctx, "/configuration", nil, &config)
}

type record struct {
	ID            int
	Title, Name   string
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	Overview      string
	PosterPath    string `json:"poster_path"`
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
	return metadata.Item{ID: r.ID, MediaType: kind, Title: title, OriginalTitle: original, Year: year, Overview: r.Overview, PosterPath: r.PosterPath}
}
func (c Client) Search(ctx context.Context, kind, query string) ([]metadata.Item, error) {
	if len(query) > 200 || strings.TrimSpace(query) == "" {
		return nil, errors.New("enter a media search term")
	}
	path := "/search/movie"
	if kind == "series" {
		path = "/search/tv"
	} else if kind != "movie" {
		return nil, errors.New("media type must be movie or series")
	}
	var result struct{ Results []record }
	if err := c.get(ctx, path, url.Values{"query": {query}, "include_adult": {"false"}}, &result); err != nil {
		return nil, err
	}
	out := []metadata.Item{}
	for _, r := range result.Results {
		out = append(out, convert(r, kind))
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
