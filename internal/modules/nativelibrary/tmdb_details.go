package nativelibrary

// TMDBDetails is a bounded, provider-only snapshot. No credentials, playback
// links or storage identifiers belong here. Episodes are obtained per season.
type TMDBDetails struct {
	StoredMovieMetadata
	Name           string        `json:"name,omitempty"`
	RuntimeMinutes int           `json:"runtimeMinutes,omitempty"`
	LogoPath       string        `json:"logoPath,omitempty"`
	Posters        []string      `json:"posters,omitempty"`
	Backdrops      []string      `json:"backdrops,omitempty"`
	Logos          []string      `json:"logos,omitempty"`
	EndDate        string        `json:"endDate,omitempty"`
	Status         string        `json:"status,omitempty"`
	People         []TMDBPerson  `json:"people,omitempty"`
	Episodes       []TMDBEpisode `json:"episodes,omitempty"`
}
type TMDBPerson struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	ImagePath string `json:"imagePath,omitempty"`
}
type TMDBEpisode struct {
	CommunityRating *float32 `json:"communityRating,omitempty"`
	ID              int64    `json:"id"`
	Season          int      `json:"season"`
	Number          int      `json:"number"`
	Name            string   `json:"name"`
	Overview        string   `json:"overview,omitempty"`
	AirDate         string   `json:"airDate,omitempty"`
	StillPath       string   `json:"stillPath,omitempty"`
	RuntimeMinutes  int      `json:"runtimeMinutes,omitempty"`
}
