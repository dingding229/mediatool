package catalog

import (
	"context"

	"errors"

	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
)

var ErrUnavailable = errors.New("catalog unavailable")
var ErrInvalid = errors.New("invalid catalog query")

type Item struct {
	ID         string `json:"id"`
	Type       string `json:"type,omitempty"`
	Source     string `json:"source"`
	Title      string `json:"title"`
	Year       string `json:"year"`
	MediaType  string `json:"mediaType"`
	Overview   string `json:"overview"`
	Poster     string `json:"poster"`
	FilePath   string `json:"filePath,omitempty"`
	State      string `json:"state,omitempty"`
	Season     *int   `json:"season,omitempty"`
	Episode    *int   `json:"episode,omitempty"`
	CategoryID string `json:"categoryId,omitempty"`
	Category   string `json:"category,omitempty"`
}
type Page struct {
	Items      []Item     `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	Source     string     `json:"source"`
	ParentID   string     `json:"parentId,omitempty"`
	Categories []Category `json:"categories,omitempty"`
}
type Engine struct {
	Sources     *SourceRegistry
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}

func (e Engine) List(ctx context.Context, source, q, kind string, page, size int) (Page, error) {
	return e.ListParent(ctx, source, q, kind, "", page, size)
}
func (e Engine) ListParent(ctx context.Context, source, q, kind, parent string, page, size int) (Page, error) {
	if parent != "" && !safeID.MatchString(parent) {
		return Page{}, ErrInvalid
	}
	out := Page{Items: []Item{}, Source: source, Page: page, PageSize: size, ParentID: parent}
	if len(q) > 200 || page < 1 || page > 10000 || size < 1 || size > 100 || kind != "" && kind != "movie" && kind != "series" {
		return out, ErrInvalid
	}
	if source != "managed" {
		return out, ErrInvalid
	}
	return e.managed(ctx, out, q, kind, "")
}

var uuidID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
var safePoster = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(?:jpg|png|webp)$`)
