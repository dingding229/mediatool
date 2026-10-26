// Package mediaserver defines the embedded native media-server boundary. A
// future TypeScript service implements the same v1 DTOs through an adapter,
// rather than coupling callers to its runtime or reusing Emby credentials.
package mediaserver

import (
	"context"
	"errors"
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
