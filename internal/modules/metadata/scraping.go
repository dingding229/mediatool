package metadata

// A setting is an output request, not permission to overwrite other files.
type ScrapeSelection struct {
	NFO      bool `json:"nfo"`
	Poster   bool `json:"poster"`
	Backdrop bool `json:"backdrop"`
	Logo     bool `json:"logo"`
	Thumb    bool `json:"thumb"`
}
type ScrapePolicy struct {
	Movie   ScrapeSelection `json:"movie"`
	Series  ScrapeSelection `json:"series"`
	Season  ScrapeSelection `json:"season"`
	Episode ScrapeSelection `json:"episode"`
}

func DefaultScrapePolicy() ScrapePolicy {
	return ScrapePolicy{Movie: ScrapeSelection{NFO: true}, Series: ScrapeSelection{NFO: true}, Episode: ScrapeSelection{NFO: true}}
}

type ScrapeNotice struct {
	Kind    string `json:"kind"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}
type Artwork struct {
	Level     string
	Role      string
	Extension string
	Episode   int
	Data      []byte
}
type SeasonInfo struct {
	ID         int
	Number     int
	Title      string
	Overview   string
	PosterPath string
}
type ScrapeBundle struct {
	Policy           ScrapePolicy
	Artworks         []Artwork
	Notices          []ScrapeNotice
	Season           *SeasonInfo
	SettingsRevision int64
}
