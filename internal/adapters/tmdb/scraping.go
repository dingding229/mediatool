package tmdb

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"media-tool/internal/modules/metadata"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

func validImagePath(p string) bool {
	return len(p) > 1 && len(p) < 2048 && strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.ContainsAny(p, "%\\?#\x00\r\n") && (strings.HasSuffix(strings.ToLower(p), ".jpg") || strings.HasSuffix(strings.ToLower(p), ".png"))
}
func (c Client) image(ctx context.Context, p string) ([]byte, string, error) {
	if !validImagePath(p) {
		return nil, "", errors.New("invalid TMDB artwork path")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/original"+p, nil)
	if err != nil {
		return nil, "", err
	}
	client := c.HTTP
	if client == nil {
		client = defaultArtworkClient()
	}
	scoped := *client
	scoped.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := scoped.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, "", errors.New("TMDB artwork request refused")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return nil, "", errors.New("artwork exceeds 8MiB limit")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 32000000 {
		return nil, "", errors.New("invalid or oversized artwork")
	}
	if _, decoded, err := image.Decode(bytes.NewReader(data)); err != nil || decoded != format {
		return nil, "", errors.New("artwork body cannot be decoded")
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	} else if format != "jpeg" {
		return nil, "", errors.New("unsupported artwork format")
	}
	return data, ext, nil
}
func defaultArtworkClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c Client) EnrichScraping(ctx context.Context, item metadata.Item, season *int, policy metadata.ScrapePolicy, revision int64) (metadata.Item, error) {
	bundle := &metadata.ScrapeBundle{Policy: policy, SettingsRevision: revision, Artworks: []metadata.Artwork{}, Notices: []metadata.ScrapeNotice{}}
	item.Scrape = bundle
	selection, level, base := policy.Movie, "movie", "/movie/"+strconv.Itoa(item.ID)
	if item.MediaType == "series" {
		selection, level, base = policy.Series, "series", "/tv/"+strconv.Itoa(item.ID)
	}
	total := 0
	add := func(level, role, p string, ep int) error {
		kind := level + "." + role
		if p == "" {
			bundle.Notices = append(bundle.Notices, metadata.ScrapeNotice{Kind: kind, State: "unavailable", Message: "TMDB未返回该图像，未创建文件"})
			return nil
		}
		data, ext, err := c.image(ctx, p)
		if err != nil {
			return err
		}
		total += len(data)
		if total > 32<<20 {
			return errors.New("total scrape artwork exceeds 32MiB")
		}
		bundle.Artworks = append(bundle.Artworks, metadata.Artwork{Level: level, Role: role, Extension: ext, Episode: ep, Data: data})
		bundle.Notices = append(bundle.Notices, metadata.ScrapeNotice{Kind: kind, State: "prepared"})
		return nil
	}
	if selection.Poster {
		if err := add(level, "poster", item.PosterPath, 0); err != nil {
			return item, err
		}
	}
	if selection.Backdrop {
		if err := add(level, "backdrop", item.BackdropPath, 0); err != nil {
			return item, err
		}
	}
	if selection.Logo {
		var reply struct {
			Logos []struct {
				Path string `json:"file_path"`
			}
		}
		if err := c.get(ctx, base+"/images", url.Values{"include_image_language": {"zh,en,null"}}, &reply); err != nil {
			return item, err
		}
		selected := ""
		if len(reply.Logos) > 0 {
			selected = reply.Logos[0].Path
		}
		if err := add(level, "logo", selected, 0); err != nil {
			return item, err
		}
	}
	if item.MediaType == "series" && season != nil {
		if policy.Season.Poster || policy.Season.NFO {
			var s struct {
				ID       int    `json:"id"`
				Number   *int   `json:"season_number"`
				Name     string `json:"name"`
				Overview string `json:"overview"`
				Poster   string `json:"poster_path"`
			}
			if err := c.get(ctx, base+"/season/"+strconv.Itoa(*season), nil, &s); err != nil {
				return item, err
			}
			if s.Number == nil || *s.Number != *season || policy.Season.NFO && s.ID < 1 {
				return item, errors.New("season identity mismatch")
			}
			bundle.Season = &metadata.SeasonInfo{ID: s.ID, Number: *season, Title: s.Name, Overview: s.Overview, PosterPath: s.Poster}
			if policy.Season.Poster {
				if err := add("season", "poster", s.Poster, 0); err != nil {
					return item, err
				}
			}
		}
		if policy.Episode.Thumb {
			for _, ep := range item.EpisodeDetails {
				if ep.Season != *season || ep.SeriesID != item.ID {
					return item, errors.New("episode artwork identity mismatch")
				}
				if err := add("episode", "thumb", ep.StillPath, ep.Number); err != nil {
					return item, err
				}
			}
		}
	}
	return item, nil
}
