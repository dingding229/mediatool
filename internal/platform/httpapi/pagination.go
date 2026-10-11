package httpapi

import (
	"errors"
	"net/http"
	"strconv"
)

type pageQuery struct {
	Page, Limit, Offset int
	Paged               bool
}

func pagination(r *http.Request) (pageQuery, error) {
	p := pageQuery{Page: 1, Limit: 50, Paged: r.URL.Query().Has("page") || r.URL.Query().Has("pageSize")}
	var err error
	if v := r.URL.Query().Get("page"); v != "" {
		p.Page, err = strconv.Atoi(v)
		if err != nil {
			return p, err
		}
	}
	if v := r.URL.Query().Get("pageSize"); v != "" {
		p.Limit, err = strconv.Atoi(v)
		if err != nil {
			return p, err
		}
	}
	if p.Page < 1 || p.Page > 20000 || p.Limit < 1 || p.Limit > 100 {
		return p, errors.New("invalid pagination")
	}
	p.Offset = (p.Page - 1) * p.Limit
	return p, nil
}
