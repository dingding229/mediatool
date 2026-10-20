package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

type pageQuery struct {
	Page, Limit, Offset int
	Paged               bool
}

func pagination(r *http.Request) (pageQuery, error) {
	p := pageQuery{Paged: r.URL.Query().Has("page") || r.URL.Query().Has("pageSize")}
	var e error
	p.Page, e = queryInt(r.URL.Query(), "page", 1, 1, 20000, false)
	if e != nil {
		return p, e
	}
	p.Limit, e = queryInt(r.URL.Query(), "pageSize", 50, 1, 100, false)
	p.Offset = (p.Page - 1) * p.Limit
	return p, e
}

// strict profiles reject duplicate/empty values; legacy page/pageSize retains
// its existing first-value/default behavior. Bounds are caller-specific.
func queryInt(q url.Values, key string, fallback, low, high int, strict bool) (int, error) {
	values, exists := q[key]
	if !exists {
		return fallback, nil
	}
	if strict && len(values) != 1 {
		return 0, errors.New("duplicate pagination")
	}
	v := q.Get(key)
	if v == "" && !strict {
		return fallback, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < low || n > high {
		return 0, errors.New("invalid pagination")
	}
	return n, nil
}
