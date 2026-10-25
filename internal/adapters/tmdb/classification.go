package tmdb

import (
	"context"
	"media-tool/internal/modules/classification"
	"strconv"
)

// ClassificationDetails requests the root read-only detail resource without
// credits/artwork appendices. Unrelated display metadata is neither validated
// nor returned by this narrowly scoped repair path.
func (c Client) ClassificationDetails(ctx context.Context, kind string, id int64) (classification.Facts, error) {
	prefix := "movie"
	if kind == "series" {
		prefix = "tv"
	} else if kind != "movie" {
		return classification.Facts{}, ErrRequest
	}
	if id < 1 {
		return classification.Facts{}, ErrRequest
	}
	var r record
	if e := c.getNativeRetry(ctx, "/"+prefix+"/"+strconv.FormatInt(id, 10), nil, &r); e != nil {
		return classification.Facts{}, e
	}
	if int64(r.ID) != id {
		return classification.Facts{}, ErrRequest
	}
	f, e := classification.FactsFromItem(convert(r, kind))
	if e != nil {
		return classification.Facts{}, ErrRequest
	}
	return f, nil
}
