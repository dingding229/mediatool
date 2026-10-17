// Package nativelibrary owns the native catalog's persistent entity vocabulary.
package nativelibrary

type EntityType string

const (
	EntityLibrary EntityType = "library"
	EntityMovie   EntityType = "movie"
	EntitySeries  EntityType = "series"
	EntitySeason  EntityType = "season"
	EntityEpisode EntityType = "episode"
)

type CollectionType string

const (
	CollectionMovies  CollectionType = "movies"
	CollectionTVShows CollectionType = "tvshows"
)

type AssetLibrarySource string

const (
	SourceClassification  AssetLibrarySource = "classification"
	SourceOutputDirectory AssetLibrarySource = "output_directory"
)

type LibraryGranularity string

const (
	GranularityRule  LibraryGranularity = "rule"
	GranularityGroup LibraryGranularity = "group"
)

type TMDBDetailType string

const (
	TMDBTypeMovie   TMDBDetailType = "movie"
	TMDBTypeSeries  TMDBDetailType = "series"
	TMDBTypeSeason  TMDBDetailType = "season"
	TMDBTypeEpisode TMDBDetailType = "episode"
)

type TMDBDetailState string

const (
	TMDBDetailPending  TMDBDetailState = "pending"
	TMDBDetailFetching TMDBDetailState = "fetching"
	TMDBDetailReady    TMDBDetailState = "ready"
	TMDBDetailFailed   TMDBDetailState = "failed"
)

const (
	FallbackMovieLibraryID  = "f:movie"
	FallbackSeriesLibraryID = "f:series"
)
