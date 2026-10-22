package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
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

// Opt-in pagination keeps non-browser/plugin callers' existing array response.
// Count SQL is a compile-time caller constant, never a browser-supplied identifier.
func servePage[T any](w http.ResponseWriter, r *http.Request, db *pgxpool.Pool, countSQL string, args []any, read func(int, int) ([]T, error)) bool {
	p, err := pagination(r)
	if err != nil || p.Offset > 200000 {
		fail(w, 400, "invalid_page", "页码或每页条数无效")
		return true
	}
	if !p.Paged {
		return false
	}
	var total int
	if err = db.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		fail(w, 503, "records_unavailable", "列表数量读取失败")
		return true
	}
	items, err := read(p.Offset, p.Limit)
	if err != nil {
		fail(w, 503, "records_unavailable", "列表读取失败")
		return true
	}
	write(w, 200, map[string]any{"items": items, "total": total, "page": p.Page, "pageSize": p.Limit})
	return true
}
