package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"media-tool/internal/modules/metadata"
)

type episodeTransport func(*http.Request) (*http.Response, error)

func (f episodeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVerifiedSeasonSubsetUsesOneRequestAndDoesNotFillMissingNumbers(t *testing.T) {
	var calls atomic.Int32
	c := Client{Token: "PRIVATE", Language: "zh-CN", HTTP: &http.Client{Transport: episodeTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != "api.themoviedb.org" || r.URL.Path != "/3/tv/99/season/0" || r.URL.Query().Get("language") != "zh-CN" || r.Header.Get("Authorization") != "Bearer PRIVATE" {
			t.Error("wrong scoped episode request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"season_number":0,"episodes":[{"id":101,"show_id":99,"season_number":0,"episode_number":1,"name":"Special one","overview":"plot one","air_date":"2026-01-01","runtime":30},{"id":103,"show_id":99,"season_number":0,"episode_number":3,"name":"Special three","overview":"plot three","air_date":null,"runtime":null}]}`))}, nil
	})}}
	episodes, err := c.EpisodeDetails(context.Background(), 99, 0, []int{3, 1})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(episodes) != 2 || episodes[0].Number != 1 || episodes[1].Number != 3 || episodes[1].Runtime != 0 || episodes[1].AirDate != "" {
		t.Fatal("wrong explicit coverage", episodes)
	}
	show, err := c.WithEpisodes(context.Background(), metadata.Item{ID: 99, MediaType: "series", Title: "Show"}, 0, []int{1})
	if err != nil || len(show.EpisodeDetails) != 1 || len(show.ShowMetadata().EpisodeDetails) != 0 {
		t.Fatal("show/asset metadata separation")
	}
	if _, err = c.EpisodeDetails(context.Background(), 99, 0, []int{2}); err == nil {
		t.Fatal("missing episode silently supplied")
	}
}
func TestEpisodeIdentityMalformedDatesDuplicatesAndHTTPFailuresFailClosed(t *testing.T) {
	valid := map[string]any{"id": 101, "show_id": 99, "season_number": 1, "episode_number": 1, "name": "owned", "air_date": "2026-01-01", "runtime": 20}
	for _, tc := range []struct {
		name, key string
		value     any
	}{{"wrong_show", "show_id", 98}, {"wrong_season", "season_number", 2}, {"missing_id", "id", 0}, {"invalid_date", "air_date", "2026-99-99"}, {"bad_runtime", "runtime", -1}, {"empty_title", "name", ""}, {"foreign_image", "still_path", "https://private.test/image"}} {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{}
			for k, v := range valid {
				item[k] = v
			}
			item[tc.key] = tc.value
			raw, _ := json.Marshal(map[string]any{"season_number": 1, "episodes": []any{item}})
			c := Client{HTTP: &http.Client{Transport: episodeTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}}
			if _, err := c.EpisodeDetails(context.Background(), 99, 1, []int{1}); !errors.Is(err, ErrRequest) {
				t.Fatal("unverified metadata accepted", err)
			}
		})
	}
	for _, body := range []string{`{"episodes":[]}`, `{"season_number":2,"episodes":[]}`, `{"season_number":1,"episodes":[{"id":1,"season_number":1,"episode_number":1,"name":"one"},{"id":2,"season_number":1,"episode_number":1,"name":"dup"}]}`} {
		c := Client{HTTP: &http.Client{Transport: episodeTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		if _, err := c.EpisodeDetails(context.Background(), 99, 1, []int{1}); err == nil {
			t.Fatal("bad response accepted")
		}
	}
	var calls atomic.Int32
	c := Client{HTTP: &http.Client{Transport: episodeTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("SECRET"))}, nil
	})}}
	if _, err := c.EpisodeDetails(context.Background(), 99, 1, []int{1}); !errors.Is(err, ErrRequest) || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("failure not sanitized")
	}
	before := calls.Load()
	if _, err := c.EpisodeDetails(context.Background(), 99, 1, []int{1, 1}); err == nil || calls.Load() != before {
		t.Fatal("invalid coverage sent to provider")
	}
}
