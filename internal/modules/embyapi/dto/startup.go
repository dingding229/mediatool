package dto

// Safe, read-only startup defaults. These contain no administrator settings,
// filesystem locations, credentials, or remotely supplied branding content.
type EndpointInfo struct {
	IsLocal     bool
	IsInNetwork bool
}
type BrandingConfiguration struct {
	LoginDisclaimer string
	CustomCss       string
}
type DisplayPreferences struct {
	Id                 string
	ViewType           string
	SortBy             string
	IndexBy            string
	RememberIndexing   bool
	PrimaryImageHeight int
	PrimaryImageWidth  int
	CustomPrefs        map[string]string
	ScrollDirection    string
	ShowBackdrop       bool
	RememberSorting    bool
	SortOrder          string
	ShowSidebar        bool
	Client             string
}

func DefaultDisplayPreferences(id string) DisplayPreferences {
	return DisplayPreferences{Id: id, ViewType: "Poster", SortBy: "SortName", PrimaryImageHeight: 250, PrimaryImageWidth: 166, CustomPrefs: map[string]string{}, ScrollDirection: "Vertical", ShowBackdrop: true, SortOrder: "Ascending", ShowSidebar: true, Client: "emby"}
}
