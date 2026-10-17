package dto

// These are discovery/taxonomy envelopes, not additional native content types.
type SearchHint struct {
	Id, ItemId, Name string
	Type             ItemType
	ProductionYear   *int   `json:",omitempty"`
	PrimaryImageTag  string `json:",omitempty"`
	ThumbImageTag    string `json:",omitempty"`
	Series           string `json:",omitempty"`
	SeriesId         string `json:",omitempty"`
}
type SearchHintResult struct {
	SearchHints      Array[SearchHint]
	TotalRecordCount int
}
type NameValuePair struct{ Name, Value string }
type ItemFilters struct {
	Genres          Array[string]
	Years           Array[int]
	OfficialRatings Array[NameValuePair]
}
