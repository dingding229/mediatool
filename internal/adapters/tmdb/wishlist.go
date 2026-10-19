package tmdb

import (
	"context"
	"errors"
	"media-tool/internal/modules/metadata"
	"net/url"
	"strconv"
)

// PublicList uses the host-owned app token. No user session/favorites are borrowed.
func (c Client) PublicList(ctx context.Context, id, page int) ([]metadata.Item, bool, error) {
	if id < 1 || page < 1 || page > 1000 {
		return nil, false, ErrRequest
	}
	var q struct {
		Items      []record `json:"items"`
		Page       int      `json:"page"`
		TotalPages int      `json:"total_pages"`
		Count      int      `json:"item_count"`
	}
	if e := c.get(ctx, "/list/"+strconv.Itoa(id), url.Values{"page": {strconv.Itoa(page)}}, &q); e != nil {
		return nil, false, e
	}
	if len(q.Items) > 200 || q.Count > len(q.Items) && q.Page == 0 {
		return nil, false, errors.New("list pagination uncertain")
	}
	// Some v3 lists are unpaged. Never pretend to advance an unpaged response.
	if q.Page != 0 && q.Page != page {
		return nil, false, ErrRequest
	}
	out := []metadata.Item{}
	for _, v := range q.Items {
		kind := v.MediaType
		if kind == "tv" {
			kind = "series"
		}
		if kind == "" && v.Title != "" && v.Name == "" {
			kind = "movie"
		}
		if v.ID <= 0 || kind != "movie" && kind != "series" {
			return nil, false, ErrRequest
		}
		out = append(out, convert(v, kind))
	}
	if q.Page == 0 {
		start := min((page-1)*20, len(out))
		end := min(start+20, len(out))
		return out[start:end], end < len(out), nil
	}
	if len(out) > 20 {
		return nil, false, ErrRequest
	}
	return out, q.Page < q.TotalPages, nil
}
