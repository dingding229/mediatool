package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	if source == "emby" {
		return e.embyList(ctx, out, q, kind)
	}
	if source != "managed" {
		return out, ErrInvalid
	}
	return e.managed(ctx, out, q, kind, "")
}

var uuidID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
var safePoster = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(?:jpg|png|webp)$`)

func (e Engine) embyRequest(ctx context.Context, route string, q url.Values) (*http.Response, error) {
	if e.Connections == nil {
		return nil, ErrUnavailable
	}
	c, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil || c.Values["apiKey"] == "" {
		return nil, ErrUnavailable
	}
	address := strings.TrimRight(c.String("endpoint"), "/") + route
	if len(q) > 0 {
		address += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("X-Emby-Token", c.Values["apiKey"])
	response, err := (&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	if response.StatusCode != 200 {
		deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
		return nil, ErrUnavailable
	}
	return response, nil
}
func (e Engine) embyList(ctx context.Context, out Page, q, kind string) (Page, error) {
	types := "Movie,Series,Episode"
	if kind == "movie" {
		types = "Movie"
	}
	if kind == "series" {
		types = "Series,Episode"
	}
	query := url.Values{"Recursive": {"true"}, "IncludeItemTypes": {types}, "Limit": {strconv.Itoa(out.PageSize)}, "StartIndex": {strconv.Itoa((out.Page - 1) * out.PageSize)}, "SearchTerm": {q}, "Fields": {"Overview"}, "SortBy": {"SortName"}, "SortOrder": {"Ascending"}}
	if out.ParentID != "" {
		query.Set("ParentId", out.ParentID)
		query.Set("Recursive", "false")
		query.Set("IncludeItemTypes", "Season,Episode")
		query.Set("SortBy", "IndexNumber")
	}
	response, err := e.embyRequest(ctx, "/Items", query)
	if err != nil {
		return out, err
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return out, ErrUnavailable
	}
	var body struct {
		Items []struct {
			ID                string            `json:"Id"`
			Name              string            `json:"Name"`
			Type              string            `json:"Type"`
			ProductionYear    int               `json:"ProductionYear"`
			Overview          string            `json:"Overview"`
			IndexNumber       *int              `json:"IndexNumber"`
			ParentIndexNumber *int              `json:"ParentIndexNumber"`
			ImageTags         map[string]string `json:"ImageTags"`
		} `json:"Items"`
		Total int `json:"TotalRecordCount"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Items == nil || body.Total < 0 || len(body.Items) > out.PageSize {
		return out, ErrUnavailable
	}
	for _, row := range body.Items {
		if !safeID.MatchString(row.ID) {
			return out, ErrUnavailable
		}
		item := Item{ID: row.ID, Source: "emby", Type: row.Type, Title: row.Name, Overview: row.Overview, Season: row.ParentIndexNumber, Episode: row.IndexNumber, State: "emby"}
		if row.Type == "Season" {
			item.Season = row.IndexNumber
			item.Episode = nil
		}
		if row.ProductionYear > 0 {
			item.Year = strconv.Itoa(row.ProductionYear)
		}
		switch row.Type {
		case "Movie":
			item.MediaType = "movie"
		case "Series", "Episode", "Season":
			item.MediaType = "series"
		default:
			return out, ErrUnavailable
		}
		if row.ImageTags["Primary"] != "" {
			item.Poster = "/api/v1/media/library/emby/" + row.ID + "/image"
		}
		out.Items = append(out.Items, item)
	}
	out.Total = body.Total
	return out, nil
}
func (e Engine) Image(ctx context.Context, id string) ([]byte, string, error) {
	if !safeID.MatchString(id) {
		return nil, "", ErrInvalid
	}
	response, err := e.embyRequest(ctx, "/Items/"+id+"/Images/Primary", url.Values{"maxWidth": {"342"}, "quality": {"85"}})
	if err != nil {
		return nil, "", err
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	content := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if content != "image/jpeg" && content != "image/png" && content != "image/webp" && content != "image/gif" {
		return nil, "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return nil, "", ErrUnavailable
	}
	return raw, content, nil
}
