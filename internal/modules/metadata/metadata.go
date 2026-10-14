package metadata

import "context"

type Item struct {
	GenreIDs         []int         `json:"genreIds,omitempty"`
	Countries        []string      `json:"countries,omitempty"`
	OriginalLanguage string        `json:"originalLanguage,omitempty"`
	Scrape           *ScrapeBundle `json:"-"`
	ID               int           `json:"id"`
	MediaType        string        `json:"mediaType"`
	Title            string        `json:"title"`
	OriginalTitle    string        `json:"originalTitle"`
	Year             string        `json:"year"`
	Overview         string        `json:"overview"`
	BackdropPath     string        `json:"backdropPath,omitempty"`
	Genres           []string      `json:"genres,omitempty"`
	PosterPath       string        `json:"posterPath"`
	EpisodeDetails   []Episode     `json:"episodeDetails,omitempty"`
}
type Provider interface {
	Search(context.Context, string, string) ([]Item, error)
	Details(context.Context, string, int) (Item, error)
}

// Episode identity is fetched for a specific series/season tuple, not inferred
// from a filename/title. Show-level metadata never stores one asset's episodes.
type Episode struct {
	ID        int    `json:"id"`
	SeriesID  int    `json:"seriesId"`
	Season    int    `json:"season"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Overview  string `json:"overview"`
	AirDate   string `json:"airDate,omitempty"`
	Runtime   int    `json:"runtimeMinutes,omitempty"`
	StillPath string `json:"stillPath,omitempty"`
}

func (i Item) ShowMetadata() Item { i.EpisodeDetails = nil; return i }
