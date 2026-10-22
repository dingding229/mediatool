package nativelibrary

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"media-tool/internal/modules/embyapi/dto"
)

type VisibleLibraries struct {
	RequestedID string
	AllowedIDs  []string
}

func (v VisibleLibraries) Valid() bool {
	if v.RequestedID == "" {
		return false
	}
	for _, id := range v.AllowedIDs {
		if id == v.RequestedID {
			return true
		}
	}
	return false
}

type HierarchyQuery struct {
	Visible           VisibleLibraries
	StartIndex, Limit int
	SortBy            string
	Descending        bool
}
type SeriesProjection struct {
	ID, ServerID, ParentID, Name, Year string
	TMDBID                             int64
	Metadata                           StoredMovieMetadata
	Details                            *TMDBDetails
	SeasonCount, EpisodeCount          int
	CreatedAt                          time.Time
}
type SeasonProjection struct {
	ID, SeriesID, ServerID, ParentID, SeriesName, Name, Year string
	TMDBID                                                   int64
	Number                                                   int
	Details                                                  *TMDBDetails
	SeriesDetails                                            *TMDBDetails
	EpisodeCount                                             int
	CreatedAt                                                time.Time
}
type EpisodeProjection struct {
	ID, SeriesID, SeasonID, ParentID, ServerID, SeriesName, SeasonName, Name, Overview, Year string
	TMDBID                                                                                   int64
	Number, NumberEnd, Season                                                                int
	AirDate                                                                                  string
	RuntimeTicks                                                                             *int64
	StillPath, LocalStillTag, FallbackPoster                                                 string
	SeriesDetails                                                                            *TMDBDetails
	Sources                                                                                  []MovieSource
	VersionOverflow                                                                          bool
	CreatedAt                                                                                time.Time
}

var ErrInvalidHierarchy = errors.New("invalid native hierarchy projection")

func SeriesDTO(in SeriesProjection) (dto.BaseItemDto, error) {
	if !numericID(in.ID) || !numericID(in.ParentID) || in.TMDBID < 1 || strings.TrimSpace(in.Name) == "" || in.CreatedAt.IsZero() {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	meta := in.Metadata
	applyDetails(&meta, in.Details)
	item, err := dto.NewBaseItemDto(in.ID, in.Name, dto.Series)
	if err != nil {
		return item, err
	}
	item.ServerId, item.ParentId = in.ServerID, in.ParentID
	item.OriginalTitle, item.SortName, item.Overview = meta.OriginalTitle, meta.SortName, meta.Overview
	if item.SortName == "" {
		item.SortName = in.Name
	}
	item.Genres = append(dto.Array[string]{}, meta.Genres...)
	item.OfficialRating, item.CommunityRating = meta.OfficialRating, meta.CommunityRating
	if item.CommunityRating != nil && (math.IsNaN(float64(*item.CommunityRating)) || math.IsInf(float64(*item.CommunityRating), 0) || *item.CommunityRating < 0 || *item.CommunityRating > 10) {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	if year := parseYear(in.Year); year != nil {
		item.ProductionYear = year
	}
	if date := parseUTCDate(meta.PremiereDate); date != nil {
		item.PremiereDate = date
	}
	if in.Details != nil {
		item.Status = in.Details.Status
		if date := parseUTCDate(in.Details.EndDate); date != nil {
			item.EndDate = date
		}
	}
	item.DateCreated = utcPtr(in.CreatedAt)
	item.ProviderIds = map[string]string{"Tmdb": strconv.FormatInt(in.TMDBID, 10)}
	if meta.ImdbID != "" {
		if !imdbIDPattern.MatchString(meta.ImdbID) {
			return dto.BaseItemDto{}, ErrInvalidHierarchy
		}
		item.ProviderIds["Imdb"] = meta.ImdbID
	}
	if meta.PosterPath != "" {
		item.ImageTags = map[string]string{"Primary": ImageTag(meta.PosterPath)}
	}
	if meta.BackdropPath != "" {
		item.BackdropImageTags = dto.Array[string]{ImageTag(meta.BackdropPath)}
	}
	addPeople(&item, in.Details)
	item.ChildCount = ptr(in.SeasonCount)
	item.RecursiveItemCount = ptr(in.EpisodeCount)
	unplayed := in.EpisodeCount
	item.UserData = &dto.UserItemDataDto{UnplayedItemCount: &unplayed}
	completeProjectionUserData(&item)
	if _, err = json.Marshal(item); err != nil {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	return item, nil
}
func SeasonDTO(in SeasonProjection) (dto.BaseItemDto, error) {
	if !numericID(in.ID) || !numericID(in.SeriesID) || !numericID(in.ParentID) || in.TMDBID < 1 || in.Number < 0 || strings.TrimSpace(in.Name) == "" || in.CreatedAt.IsZero() {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	item, err := dto.NewBaseItemDto(in.ID, in.Name, dto.Season)
	if err != nil {
		return item, err
	}
	item.ServerId, item.ParentId, item.SeriesId, item.SeriesName = in.ServerID, in.ParentID, in.SeriesID, in.SeriesName
	item.IndexNumber = ptr(in.Number)
	item.ProductionYear = parseYear(in.Year)
	if in.SeriesDetails != nil && in.SeriesDetails.PosterPath != "" {
		item.SeriesPrimaryImageTag = ImageTag(in.SeriesDetails.PosterPath)
	}
	item.DateCreated = utcPtr(in.CreatedAt)
	item.ChildCount = ptr(in.EpisodeCount)
	unplayed := in.EpisodeCount
	item.UserData = &dto.UserItemDataDto{UnplayedItemCount: &unplayed}
	if in.Details != nil {
		if in.Details.Name != "" {
			item.Name = in.Details.Name
		}
		if date := parseUTCDate(in.Details.PremiereDate); date != nil {
			item.PremiereDate = date
		}
		if in.Details.PosterPath != "" {
			item.ImageTags = map[string]string{"Primary": ImageTag(in.Details.PosterPath)}
		}
	}
	completeProjectionUserData(&item)
	if _, err = json.Marshal(item); err != nil {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	return item, nil
}
func EpisodeDTO(in EpisodeProjection) (dto.BaseItemDto, error) {
	if !numericID(in.ID) || !numericID(in.SeriesID) || !numericID(in.SeasonID) || !numericID(in.ParentID) || in.TMDBID < 1 || in.Number < 1 || in.Season < 0 || strings.TrimSpace(in.Name) == "" || in.CreatedAt.IsZero() {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	item, err := dto.NewBaseItemDto(in.ID, in.Name, dto.Episode)
	if err != nil {
		return item, err
	}
	item.ServerId, item.ParentId, item.SeriesId, item.SeasonId, item.SeriesName, item.SeasonName = in.ServerID, in.ParentID, in.SeriesID, in.SeasonID, in.SeriesName, in.SeasonName
	item.CollectionType = "tvshows"
	item.Overview = in.Overview
	item.IndexNumber = ptr(in.Number)
	if in.NumberEnd > in.Number {
		item.IndexNumberEnd = ptr(in.NumberEnd)
	}
	item.ParentIndexNumber = ptr(in.Season)
	item.DateCreated = utcPtr(in.CreatedAt)
	item.RunTimeTicks = in.RuntimeTicks
	if date := parseUTCDate(in.AirDate); date != nil {
		item.PremiereDate = date
	}
	item.Path = SyntheticFilename(in.Name, firstContainer(in.Sources))
	item.MediaSources = append(dto.Array[dto.MediaSourceInfo]{}, mediaSources(in.Sources, item.Name)...)
	applyProbeItem(&item, in.Sources)
	if in.VersionOverflow && len(item.MediaSources) > 0 {
		item.MediaSources[0].Name += "（仅展示最新100个版本）"
	}
	if in.SeriesDetails != nil {
		if in.SeriesDetails.PosterPath != "" {
			item.SeriesPrimaryImageTag = ImageTag(in.SeriesDetails.PosterPath)
		}
		if in.SeriesDetails.BackdropPath != "" {
			item.ParentBackdropImageTags = dto.Array[string]{ImageTag(in.SeriesDetails.BackdropPath)}
		}
	}
	item.ParentBackdropItemId = in.SeriesID
	if in.StillPath != "" {
		item.ImageTags = map[string]string{"Primary": ImageTag(in.StillPath)}
	} else if in.LocalStillTag != "" {
		item.ImageTags = map[string]string{"Primary": in.LocalStillTag}
	} else if in.FallbackPoster != "" {
		item.ImageTags = map[string]string{"Primary": ImageTag(in.FallbackPoster)}
	}
	completeProjectionUserData(&item)
	if _, err = json.Marshal(item); err != nil {
		return dto.BaseItemDto{}, ErrInvalidHierarchy
	}
	return item, nil
}
func applyDetails(meta *StoredMovieMetadata, d *TMDBDetails) {
	if d == nil {
		return
	}
	for _, p := range []struct {
		dst *string
		src string
	}{{&meta.OriginalTitle, d.OriginalTitle}, {&meta.Overview, d.Overview}, {&meta.PosterPath, d.PosterPath}, {&meta.BackdropPath, d.BackdropPath}, {&meta.PremiereDate, d.PremiereDate}, {&meta.OfficialRating, d.OfficialRating}, {&meta.ImdbID, d.ImdbID}} {
		if p.src != "" {
			*p.dst = p.src
		}
	}
	if len(d.Genres) > 0 {
		meta.Genres = append([]string(nil), d.Genres...)
	}
	if d.CommunityRating != nil {
		meta.CommunityRating = d.CommunityRating
	}
}
func addPeople(item *dto.BaseItemDto, d *TMDBDetails) {
	if d == nil {
		return
	}
	for _, p := range d.People[:min(len(d.People), 60)] {
		if p.ID < 1 || p.Name == "" {
			continue
		}
		x := dto.BaseItemPerson{NameIdPair: dto.NameIdPair{Id: strconv.FormatInt(p.ID, 10), Name: p.Name}, Type: p.Type, Role: p.Role}
		if p.ImagePath != "" {
			x.PrimaryImageTag = ImageTag(p.ImagePath)
		}
		item.People = append(item.People, x)
	}
}
func mediaSources(sources []MovieSource, name string) []dto.MediaSourceInfo {
	out := make([]dto.MediaSourceInfo, 0, len(sources))
	for _, s := range sources {
		x, _ := dto.NewMediaSourceInfo("")
		x.Id = s.ID
		x.Name = name
		x.Container = strings.ToLower(s.Container)
		x.RunTimeTicks = s.RunTimeTicks
		applyProbeSource(&x, s.Probe)
		out = append(out, x)
	}
	return out
}
func firstContainer(s []MovieSource) string {
	if len(s) > 0 && s[0].Container != "" {
		return strings.ToLower(s[0].Container)
	}
	return "mkv"
}
func SyntheticFilename(name, extension string) string {
	if extension == "" {
		extension = "mkv"
	}
	leaf := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\\:?&=#%`, r) {
			return ' '
		}
		return r
	}, name)
	for strings.Contains(leaf, "..") {
		leaf = strings.ReplaceAll(leaf, "..", " ")
	}
	leaf = strings.Join(strings.Fields(leaf), " ")
	r := []rune(leaf)
	if len(r) > 100 {
		leaf = string(r[:100])
	}
	if leaf == "" {
		return ""
	}
	return leaf + "." + extension
}
func parseYear(s string) *int {
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || n > 9999 {
		return nil
	}
	return &n
}
func parseUTCDate(s string) *dto.UTCDateTime {
	if s == "" {
		return nil
	}
	d, e := time.Parse("2006-01-02", s)
	if e != nil {
		return nil
	}
	v := dto.NewUTCDateTime(d)
	return &v
}
func utcPtr(t time.Time) *dto.UTCDateTime { v := dto.NewUTCDateTime(t); return &v }
func ptr[T any](v T) *T                   { return &v }
