package nativelibrary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var imdbIDPattern = regexp.MustCompile(`^tt[0-9]{1,20}$`)
var containerPattern = regexp.MustCompile(`^[a-zA-Z0-9]{1,16}$`)

var ErrInvalidProjection = errors.New("invalid native media projection")

// StoredMovieMetadata describes optional data already persisted in media_items.
// Missing values stay absent; projection never fetches metadata or media.
type StoredMovieMetadata struct {
	OriginalTitle   string   `json:"originalTitle"`
	SortName        string   `json:"sortName"`
	Overview        string   `json:"overview"`
	Genres          []string `json:"genres"`
	PosterPath      string   `json:"posterPath"`
	BackdropPath    string   `json:"backdropPath"`
	PremiereDate    string   `json:"premiereDate"`
	CommunityRating *float32 `json:"communityRating"`
	OfficialRating  string   `json:"officialRating"`
	ImdbID          string   `json:"imdbId"`
}
type MovieSource struct {
	ID, Container string
	RunTimeTicks  *int64
	Probe         *probeinfo.Result `json:"-"`
}
type Movie struct {
	ID, ServerID, ParentID, Name, Year string
	TMDBID                             int64
	Metadata                           StoredMovieMetadata
	CreatedAt                          time.Time
	RunTimeTicks                       *int64
	Sources                            []MovieSource
	Details                            *TMDBDetails
}

func StillImageTag(fingerprint, digest string) string {
	if len(fingerprint) != 64 || len(digest) != 64 {
		return ""
	}
	return ImageTag("native-still:" + fingerprint + ":" + digest)
}
func ImageTag(tmdbPath string) string {
	sum := sha256.Sum256([]byte(tmdbPath))
	return hex.EncodeToString(sum[:])
}
func LibraryDTO(l Library) (dto.BaseItemDto, error) {
	if !numericID(l.ItemID) || l.Name == "" || (l.CollectionType != CollectionMovies && l.CollectionType != CollectionTVShows) {
		return dto.BaseItemDto{}, ErrInvalidProjection
	}
	item, err := dto.NewBaseItemDto(l.ItemID, l.Name, dto.CollectionFolder)
	if err != nil {
		return item, err
	}
	item.ServerId = l.ServerID
	item.CollectionType = string(l.CollectionType)
	if _, err := json.Marshal(item); err != nil {
		return dto.BaseItemDto{}, ErrInvalidProjection
	}
	return item, nil
}
func MovieDTO(movie Movie) (dto.BaseItemDto, error) {
	movie = mergeMovieDetails(movie)
	if !numericID(movie.ID) || !numericID(movie.ParentID) || movie.TMDBID < 1 || strings.TrimSpace(movie.Name) == "" || movie.CreatedAt.IsZero() {
		return dto.BaseItemDto{}, ErrInvalidProjection
	}
	item, err := dto.NewBaseItemDto(movie.ID, movie.Name, dto.Movie)
	if err != nil {
		return item, err
	}
	item.ServerId = movie.ServerID
	item.ParentId = movie.ParentID
	item.OriginalTitle = movie.Metadata.OriginalTitle
	item.SortName = movie.Metadata.SortName
	if item.SortName == "" {
		item.SortName = movie.Name
	}
	if movie.Details != nil {
		if movie.Details.LogoPath != "" {
			item.ImageTags = map[string]string{"Logo": ImageTag(movie.Details.LogoPath)}
		}
		for _, person := range movie.Details.People[:min(len(movie.Details.People), 60)] {
			p := dto.BaseItemPerson{NameIdPair: dto.NameIdPair{Id: strconv.FormatInt(person.ID, 10), Name: person.Name}, Type: person.Type, Role: person.Role}
			if person.ImagePath != "" {
				p.PrimaryImageTag = ImageTag(person.ImagePath)
			}
			item.People = append(item.People, p)
		}
	}
	item.Overview = movie.Metadata.Overview
	item.Genres = append(dto.Array[string]{}, movie.Metadata.Genres...)
	item.OfficialRating = movie.Metadata.OfficialRating
	item.CommunityRating = movie.Metadata.CommunityRating
	if item.CommunityRating != nil && (math.IsNaN(float64(*item.CommunityRating)) || math.IsInf(float64(*item.CommunityRating), 0) || *item.CommunityRating < 0 || *item.CommunityRating > 10) {
		return dto.BaseItemDto{}, ErrInvalidProjection
	}
	if movie.Year != "" {
		year, e := strconv.Atoi(movie.Year)
		if e != nil || year < 1 || year > 9999 {
			return dto.BaseItemDto{}, ErrInvalidProjection
		}
		item.ProductionYear = &year
	}
	if movie.Metadata.PremiereDate != "" {
		date, e := time.Parse("2006-01-02", movie.Metadata.PremiereDate)
		if e != nil {
			return dto.BaseItemDto{}, ErrInvalidProjection
		}
		utc := dto.NewUTCDateTime(date)
		item.PremiereDate = &utc
	}
	created := dto.NewUTCDateTime(movie.CreatedAt)
	item.DateCreated = &created
	item.RunTimeTicks = movie.RunTimeTicks
	item.UserData = &dto.UserItemDataDto{}
	item.ProviderIds = map[string]string{"Tmdb": strconv.FormatInt(movie.TMDBID, 10)}
	if movie.Metadata.ImdbID != "" {
		if !imdbIDPattern.MatchString(movie.Metadata.ImdbID) {
			return dto.BaseItemDto{}, ErrInvalidProjection
		}
		item.ProviderIds["Imdb"] = movie.Metadata.ImdbID
	}
	if movie.Metadata.PosterPath != "" {
		if item.ImageTags == nil {
			item.ImageTags = map[string]string{}
		}
		item.ImageTags["Primary"] = ImageTag(movie.Metadata.PosterPath)
	}
	if movie.Metadata.BackdropPath != "" {
		item.BackdropImageTags = dto.Array[string]{ImageTag(movie.Metadata.BackdropPath)}
	}
	extension := "mkv"
	for index, source := range movie.Sources {
		media, err := dto.NewMediaSourceInfo("")
		if err != nil {
			return dto.BaseItemDto{}, err
		}
		if source.ID == "" {
			return dto.BaseItemDto{}, ErrInvalidProjection
		}
		media.Id = source.ID
		media.Name = fmt.Sprintf("版本 %d", index+1)
		if source.Container != "" {
			if !containerPattern.MatchString(source.Container) {
				return dto.BaseItemDto{}, ErrInvalidProjection
			}
			media.Container = strings.ToLower(source.Container)
			if index == 0 {
				extension = media.Container
			}
		}
		media.RunTimeTicks = source.RunTimeTicks
		if media.RunTimeTicks == nil {
			media.RunTimeTicks = movie.RunTimeTicks
		}
		applyProbeSource(&media, source.Probe)
		item.MediaSources = append(item.MediaSources, media)
	}
	applyProbeItem(&item, movie.Sources)
	leaf := SyntheticFilename(movie.Name, extension)
	item.Path = leaf
	if _, err := json.Marshal(item); err != nil {
		return dto.BaseItemDto{}, ErrInvalidProjection
	}
	return item, nil
}
func numericID(value string) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number >= 100000 && strconv.FormatInt(number, 10) == value
}

// Snapshot merge is pure. Probe ticks win; only known provider values replace
// persisted metadata. Missing/failed fetches never invoke a provider here.
func mergeMovieDetails(movie Movie) Movie {
	d := movie.Details
	if d == nil {
		return movie
	}
	m := &movie.Metadata
	for _, pair := range []struct {
		dst *string
		src string
	}{{&m.OriginalTitle, d.OriginalTitle}, {&m.Overview, d.Overview}, {&m.PosterPath, d.PosterPath}, {&m.BackdropPath, d.BackdropPath}, {&m.PremiereDate, d.PremiereDate}, {&m.OfficialRating, d.OfficialRating}, {&m.ImdbID, d.ImdbID}} {
		if pair.src != "" {
			*pair.dst = pair.src
		}
	}
	if len(d.Genres) > 0 {
		m.Genres = append([]string(nil), d.Genres...)
	}
	if d.CommunityRating != nil {
		m.CommunityRating = d.CommunityRating
	}
	if movie.RunTimeTicks == nil && d.RuntimeMinutes > 0 && d.RuntimeMinutes <= 1440 {
		ticks := int64(d.RuntimeMinutes) * 60 * 10000000
		movie.RunTimeTicks = &ticks
	}
	return movie
}
