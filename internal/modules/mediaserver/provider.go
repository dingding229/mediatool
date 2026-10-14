// Package mediaserver defines the embedded native media-server boundary. A
// future TypeScript service implements the same v1 DTOs through an adapter,
// rather than coupling callers to its runtime or reusing Emby credentials.
package mediaserver

import (
	"context"
	"errors"
	"media-tool/internal/modules/catalog"
)

var (
	ErrUnsupported = errors.New("media server capability reserved")
	ErrDenied      = errors.New("media library access denied")
	ErrUnavailable = errors.New("media server unavailable")
	ErrInvalid     = errors.New("invalid media query")
	ErrNotFound    = errors.New("media item not found")
)

const ContractVersion = "v1"

type Capabilities struct {
	Catalog            bool `json:"catalog"`
	MemberAuth         bool `json:"memberAuth"`
	LibraryPermissions bool `json:"libraryPermissions"`
	PlaybackTickets    bool `json:"playbackTickets"`
	Progress           bool `json:"progress"`
}
type Description struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Contract     string       `json:"contract"`
	Runtime      string       `json:"runtime"`
	Stage        string       `json:"stage"`
	Capabilities Capabilities `json:"capabilities"`
	Note         string       `json:"note"`
}
type Library struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	CanPlay   bool   `json:"canPlay"`
}

// Access is server-created. It must never be decoded from the client's body.
type Access struct {
	UserID   string
	Revision int64
}
type Query struct {
	LibraryID, Text string
	Page, PageSize  int
}
type Item struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Year      string `json:"year"`
	MediaType string `json:"mediaType"`
	Overview  string `json:"overview"`
	Poster    string `json:"poster"`
	Season    *int   `json:"season,omitempty"`
	Episode   *int   `json:"episode,omitempty"`
}
type Page struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type PlaybackRequest struct{ LibraryID, ItemID, UserAgent, DeviceID string }

// PlaybackTicket is deliberately distinct from persistent administrator/file
// grants. No ticket is issued before short-lived revocation is implemented.
type PlaybackTicket struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
	Delivery  string `json:"delivery"`
}
type Provider interface {
	Describe() Description
	Libraries(context.Context, Access) ([]Library, error)
	List(context.Context, Access, Query) (Page, error)
	Playback(context.Context, Access, PlaybackRequest) (PlaybackTicket, error)
}
type Registry struct {
	ordered []Provider
	byID    map[string]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{byID: map[string]Provider{}}
	for _, p := range providers {
		if p == nil {
			return nil, ErrInvalid
		}
		d := p.Describe()
		if d.ID == "" || d.Contract != ContractVersion || r.byID[d.ID] != nil {
			return nil, ErrInvalid
		}
		r.byID[d.ID] = p
		r.ordered = append(r.ordered, p)
	}
	return r, nil
}
func (r *Registry) Provider(id string) (Provider, error) {
	p := r.byID[id]
	if p == nil {
		return nil, ErrNotFound
	}
	return p, nil
}
func (r *Registry) Descriptions() []Description {
	out := []Description{}
	for _, p := range r.ordered {
		out = append(out, p.Describe())
	}
	return out
}

// EmbyAdapter exposes administrative catalog capability without delegating the
// member realm to Emby's API Key. Member access is deliberately rejected.
type EmbyAdapter struct{ Catalog catalog.Engine }

func (EmbyAdapter) Describe() Description {
	return Description{ID: "emby", Name: "Emby", Contract: ContractVersion, Runtime: "external", Stage: "external", Capabilities: Capabilities{Catalog: true}, Note: "现有外部媒体库保持不变；连接配置仍在媒体库页面。此适配器不转授Emby管理员密钥，也不提供媒体用户播放票据。"}
}
func (e EmbyAdapter) Libraries(ctx context.Context, a Access) ([]Library, error) {
	return nil, ErrUnsupported
}
func (e EmbyAdapter) List(ctx context.Context, a Access, q Query) (Page, error) {
	if a.UserID != "" {
		return Page{}, ErrDenied
	}
	p, err := e.Catalog.ListParent(ctx, "emby", q.Text, "", q.LibraryID, q.Page, q.PageSize)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	out := Page{Items: []Item{}, Total: p.Total, Page: p.Page, PageSize: p.PageSize}
	for _, i := range p.Items {
		out.Items = append(out.Items, Item{ID: i.ID, Title: i.Title, Year: i.Year, MediaType: i.MediaType, Overview: i.Overview, Poster: i.Poster, Season: i.Season, Episode: i.Episode})
	}
	return out, nil
}
func (EmbyAdapter) Playback(context.Context, Access, PlaybackRequest) (PlaybackTicket, error) {
	return PlaybackTicket{}, ErrUnsupported
}
