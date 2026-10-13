package sites

import "context"

type Resource struct {
	Partial        bool     `json:"-"`
	SiteID         string   `json:"siteId"`
	TorrentID      string   `json:"torrentId"`
	Title          string   `json:"title"`
	Subtitle       string   `json:"subtitle"`
	DetailsURL     string   `json:"detailsUrl"`
	DownloadURL    string   `json:"-"`
	Size           int64    `json:"size"`
	Seeders        int      `json:"seeders"`
	Leechers       int      `json:"leechers"`
	HR             *bool    `json:"hr"`
	DownloadFactor *float64 `json:"downloadFactor"`
	Labels         []string `json:"labels"`
}
type SearchQuery struct {
	Keyword string
	Page    int
}
type SiteAdapter interface {
	Test(context.Context) error
	Search(context.Context, SearchQuery) ([]Resource, error)
	Torrent(context.Context, string, string) ([]byte, error)
}
type Definition struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Schema              string         `json:"schema"`
	SourceRepository    string         `json:"sourceRepository"`
	SourceCommit        string         `json:"sourceCommit"`
	Compatibility       map[string]any `json:"compatibility"`
	RawJSON, CommonJSON []byte         `json:"-"`
}
