package tmdb

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"media-tool/internal/modules/metadata"
	"net/http"
	"strings"
	"testing"
)

type artworkTransport func(*http.Request) (*http.Response, error)

func (f artworkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestScrapingFetchesOnlySelectedBoundedArtwork(t *testing.T) {
	var imageData bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	png.Encode(&imageData, img)
	calls := []string{}
	client := Client{Token: "owned", HTTP: &http.Client{Transport: artworkTransport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.Host+r.URL.Path)
		body := []byte(`{"logos":[]}`)
		if r.URL.Host == "image.tmdb.org" {
			if r.Header.Get("Authorization") != "" {
				t.Error("API key sent to image origin")
			}
			body = imageData.Bytes()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}}
	item := metadata.Item{ID: 1, MediaType: "movie", Title: "Owned", PosterPath: "/owned.png"}
	out, err := client.EnrichScraping(context.Background(), item, nil, metadata.DefaultScrapePolicy(), 1)
	if err != nil || len(calls) != 0 || len(out.Scrape.Artworks) != 0 {
		t.Fatal(out, err, calls)
	}
	policy := metadata.DefaultScrapePolicy()
	policy.Movie.Poster = true
	policy.Movie.Logo = true
	out, err = client.EnrichScraping(context.Background(), item, nil, policy, 2)
	if err != nil || len(out.Scrape.Artworks) != 1 || out.Scrape.Artworks[0].Extension != ".png" || out.Scrape.Notices[1].State != "unavailable" {
		t.Fatal(out, err)
	}
	for _, p := range []string{"https://evil.invalid/img.png", "/../a.png", "//evil.png", "/a.png?key=bad", "/a.jpg\x00"} {
		if validImagePath(p) {
			t.Fatal(p)
		}
	}
	client.HTTP = &http.Client{Transport: artworkTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("not an image"))}, nil
	})}
	if _, err = client.EnrichScraping(context.Background(), item, nil, policy, 2); err == nil {
		t.Fatal("invalid image staged")
	}
}

func TestSeasonAndEpisodeArtworkRequireTheirOwnVerifiedMetadata(t *testing.T) {
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	c := Client{Token: "owned", HTTP: &http.Client{Transport: artworkTransport(func(r *http.Request) (*http.Response, error) {
		raw := data.Bytes()
		if r.URL.Host == "api.themoviedb.org" {
			raw = []byte(`{"id":55,"season_number":1,"name":"Season One","overview":"owned","poster_path":"/season.png"}`)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}}
	policy := metadata.DefaultScrapePolicy()
	policy.Season.NFO = true
	policy.Season.Poster = true
	policy.Episode.Thumb = true
	s := 1
	item := metadata.Item{ID: 10, Title: "Owned", MediaType: "series", EpisodeDetails: []metadata.Episode{{ID: 101, SeriesID: 10, Season: 1, Number: 1, StillPath: "/episode.png"}}}
	out, err := c.EnrichScraping(context.Background(), item, &s, policy, 1)
	if err != nil || out.Scrape.Season.ID != 55 || len(out.Scrape.Artworks) != 2 {
		t.Fatal(out, err)
	}
	item.EpisodeDetails[0].SeriesID = 11
	if _, err = c.EnrichScraping(context.Background(), item, &s, policy, 1); err == nil {
		t.Fatal("other show thumb selected")
	}
}
