package nativelibrary

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"time"
)

var (
	ErrNotImplemented = errors.New("native library not implemented")
	ErrNotFound       = errors.New("native item not found")
)

// Models are internal projections, not directly serializable protocol DTOs.
type Library struct {
	ID, ItemID, GroupID, RuleID, Name, ServerID string
	CollectionType                              CollectionType
	Hidden                                      bool
	PolicyRevision                              int64
}
type Hierarchy struct {
	ParentID, SeriesID, SeasonID, SeriesName                       string
	IndexNumber, ParentIndexNumber, ChildCount, RecursiveItemCount *int
}
type Item struct {
	Hierarchy
	ID, LibraryID, Title, OriginalTitle, Overview string
	Type                                          EntityType
	Year, ParentalRating                          *int
	RuntimeTicks                                  *int64
	Genres                                        []string
}

// Every field corresponds one-to-one to embyapi.Query. No raw SQL fragments.
type ItemQuery struct {
	StartIndex                                                         int64
	Limit                                                              int
	SortBy, SortOrder, Fields, IDs, IncludeItemTypes, ExcludeItemTypes []string
	ParentID, SeriesID, SeasonID string
	Genres, Years                                                      []string
	SearchTerm                                                         string
	Recursive, IsFolder, IsPlayed, IsFavorite, IsResumable             *bool
}

type EpisodeQuery struct {
	Visible                     VisibleLibraries
	SeriesID, SeasonID          string
	StartIndex, Limit           int
	TargetSeason, TargetEpisode *int
}
type HierarchyService interface {
	ListSeries(context.Context, HierarchyQuery) ([]dto.BaseItemDto, int, error)
	GetSeries(context.Context, VisibleLibraries, string) (dto.BaseItemDto, error)
	ListSeasons(context.Context, VisibleLibraries, string) ([]dto.BaseItemDto, error)
	ListEpisodes(context.Context, EpisodeQuery) ([]dto.BaseItemDto, int, error)
	GetEpisode(context.Context, EpisodeQuery, string) (dto.BaseItemDto, error)
}
type UserData struct {
	PositionTicks    int64
	PlayCount        int
	Played, Favorite bool
	LastPlayedAt     *time.Time
}
type UserDataPatch struct {
	PositionTicks    *int64
	Played, Favorite *bool
}
type LibraryService interface {
	ListLibraries(context.Context, contracts.User) ([]Library, error)
	GetLibrary(context.Context, contracts.User, string) (Library, error)
}
type ItemService interface {
	Query(context.Context, contracts.User, ItemQuery) ([]Item, int, error)
	Get(context.Context, contracts.User, string) (Item, error)
	Seasons(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error)
	Episodes(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error)
	Latest(context.Context, contracts.User, ItemQuery) ([]Item, int, error)
	Search(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error)
}
type UserDataService interface {
	Get(context.Context, contracts.User, string) (UserData, error)
	Update(context.Context, contracts.User, string, UserDataPatch) (UserData, error)
	MarkPlayed(context.Context, contracts.User, string, bool) (UserData, error)
	MarkFavorite(context.Context, contracts.User, string, bool) (UserData, error)
	Resume(context.Context, contracts.User, ItemQuery) ([]Item, int, error)
	NextUp(context.Context, contracts.User, ItemQuery) ([]Item, int, error)
}
type emptyLibrary struct{}
type emptyItems struct{}
type emptyUserData struct{}

func NewLibraryService() LibraryService   { return emptyLibrary{} }
func NewItemService() ItemService         { return emptyItems{} }
func NewUserDataService() UserDataService { return emptyUserData{} }
func (emptyLibrary) ListLibraries(context.Context, contracts.User) ([]Library, error) {
	return []Library{}, ErrNotImplemented
}
func (emptyLibrary) GetLibrary(context.Context, contracts.User, string) (Library, error) {
	return Library{}, ErrNotImplemented
}
func (emptyItems) Query(context.Context, contracts.User, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyItems) Get(context.Context, contracts.User, string) (Item, error) {
	return Item{}, ErrNotImplemented
}
func (emptyItems) Seasons(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyItems) Episodes(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyItems) Latest(context.Context, contracts.User, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyItems) Search(context.Context, contracts.User, string, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyUserData) Get(context.Context, contracts.User, string) (UserData, error) {
	return UserData{}, ErrNotImplemented
}
func (emptyUserData) Update(context.Context, contracts.User, string, UserDataPatch) (UserData, error) {
	return UserData{}, ErrNotImplemented
}
func (emptyUserData) MarkPlayed(context.Context, contracts.User, string, bool) (UserData, error) {
	return UserData{}, ErrNotImplemented
}
func (emptyUserData) MarkFavorite(context.Context, contracts.User, string, bool) (UserData, error) {
	return UserData{}, ErrNotImplemented
}
func (emptyUserData) Resume(context.Context, contracts.User, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}
func (emptyUserData) NextUp(context.Context, contracts.User, ItemQuery) ([]Item, int, error) {
	return []Item{}, 0, ErrNotImplemented
}

// Mapping signatures only. P1 supplies the actual metadata projections.
func ItemDTO(Item) (dto.BaseItemDto, error) { return dto.BaseItemDto{}, ErrNotImplemented }
func UserDataDTO(UserData) (dto.UserItemDataDto, error) {
	return dto.UserItemDataDto{}, ErrNotImplemented
}
