package tmdb

import (
	"context"
	"fmt"
)

// ImageCandidates is called only by an explicit administrator picker operation.
func (c Client) ImageCandidates(ctx context.Context, kind string, id int64, season, episode int) (map[string][]string, error) {
	endpoint := fmt.Sprintf("/movie/%d/images", id)
	switch kind {
	case "movie":
	case "series":
		endpoint = fmt.Sprintf("/tv/%d/images", id)
	case "season":
		endpoint = fmt.Sprintf("/tv/%d/season/%d/images", id, season)
	case "episode":
		endpoint = fmt.Sprintf("/tv/%d/season/%d/episode/%d/images", id, season, episode)
	default:
		return nil, ErrRequest
	}
	var raw struct {
		Posters, Backdrops, Logos, Stills []struct {
			Path string `json:"file_path"`
		}
	}
	if e := c.getNativeRetry(ctx, endpoint, nil, &raw); e != nil {
		return nil, e
	}
	out := map[string][]string{"Primary": {}, "Backdrop": {}, "Logo": {}}
	for _, group := range []struct {
		kind  string
		paths []struct {
			Path string `json:"file_path"`
		}
	}{{"Primary", raw.Posters}, {"Primary", raw.Stills}, {"Backdrop", raw.Backdrops}, {"Logo", raw.Logos}} {
		for _, p := range group.paths {
			if nativeImagePath(p.Path) && len(out[group.kind]) < 100 {
				out[group.kind] = append(out[group.kind], p.Path)
			}
		}
	}
	return out, nil
}
