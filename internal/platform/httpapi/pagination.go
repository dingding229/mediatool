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
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return p, e
	}
	p.Page, e = queryInt(q, "page", 1, 1, 20000)
	if e != nil {
		return p, e
	}
	p.Limit, e = queryInt(q, "pageSize", 50, 1, 100)
	p.Offset = (p.Page - 1) * p.Limit
	return p, e
}

// All profiles reject duplicate/empty values. Bounds remain caller-specific.
func queryInt(q url.Values, key string, fallback, low, high int) (int, error) {
	values, exists := q[key]
	if !exists {
		return fallback, nil
	}
	if len(values) != 1 {
		return 0, errors.New("duplicate pagination")
	}
	v := q.Get(key)
	n, e := strconv.Atoi(v)
	if e != nil || n < low || n > high {
		return 0, errors.New("invalid pagination")
	}
	return n, nil
}
