package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

func (r BrowseRepository) single(ctx context.Context, u contracts.User, p contracts.PermissionChecker, id string, kind dto.ItemType) (dto.BaseItemDto, error) {
	// Parent/Similar preflight only needs identity, genres and permission checks.
	// Do not spend a probe snapshot query outside the actual returned page.
	items, _, err := r.query(ctx, u, p, nativelibrary.ItemQuery{IDs: []string{id}, Limit: 1}, browseOptions{skipProbes: true, skipCount: true})
	if err != nil {
		return dto.BaseItemDto{}, err
	}
	if len(items) != 1 || kind != "" && items[0].Type != kind {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	return items[0], nil
}
func (r BrowseRepository) Discover(ctx context.Context, u contracts.User, p contracts.PermissionChecker, in nativelibrary.DiscoveryQuery) (nativelibrary.DiscoveryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := nativelibrary.DiscoveryResult{Items: []dto.BaseItemDto{}, Genres: []dto.NameIdPair{}}
	q := in.Items
	value := true
	q.Recursive = &value
	if len(in.ItemID) > 64 || len(in.SeasonID) > 64 || len(in.StartItemID) > 64 || in.Season != nil && (*in.Season < 0 || *in.Season > 10000) {
		return out, nativelibrary.ErrInvalidQuery
	}
	if err := validateBrowseQuery(&q); err != nil {
		return out, err
	}
	options := browseOptions{}
	restrict := func(types ...string) {
		if len(q.IncludeItemTypes) == 0 {
			q.IncludeItemTypes = types
			return
		}
		matches := []string{}
		for _, input := range q.IncludeItemTypes {
			for _, kind := range types {
				if strings.EqualFold(input, kind) {
					matches = append(matches, kind)
					break
				}
			}
		}
		if len(matches) == 0 {
			matches = []string{"__unsupported__"}
		}
		q.IncludeItemTypes = matches
	}
	switch in.Mode {
	case nativelibrary.Resume:
		if len(q.IncludeItemTypes) == 0 {
			q.IncludeItemTypes = []string{"Movie", "Episode"}
		}
		if len(in.MediaTypes) > 8 {
			return out, nativelibrary.ErrInvalidQuery
		}
		if len(in.MediaTypes) > 0 {
			types := []string{}
			for _, t := range in.MediaTypes {
				switch strings.ToLower(t) {
				case "video":
					types = append(types, "Movie", "Episode")
				case "movie":
					types = append(types, "Movie")
				case "episode":
					types = append(types, "Episode")
				default: // Unknown media kinds contribute no matches, just like unsupported standard kinds.
				}
			}
			if len(types) == 0 {
				options.extra = func(w string, a []any) (string, []any) { return w + " AND false", a }
			} else {
				restrict(types...)
			}
		}
		q.IsResumable = &value
		options.order = []string{"user_last_played DESC NULLS LAST"}
	case nativelibrary.NextUp:
		q.SeriesID, q.SeasonID = "", "" // The optimized NextUp path already binds and verifies in.ItemID.
		q.IDs = nil
		q.Genres = nil
		q.Years = nil
		q.SearchTerm = ""
		q.IsFolder = nil
		restrict("Episode")
		if len(q.IncludeItemTypes) != 1 || q.IncludeItemTypes[0] != "Episode" {
			return out, nil
		}
		for _, kind := range q.ExcludeItemTypes {
			if strings.EqualFold(kind, "Episode") {
				return out, nil
			}
		}
		q.ExcludeItemTypes = nil
		q.IsPlayed = nil
		q.IsResumable = nil
		q.IsFavorite = nil
		options.nextUp = true
		options.nextSeries = in.ItemID
		options.nextSpecial = in.Special != nil && *in.Special

	case nativelibrary.Seasons, nativelibrary.Episodes:
		show, err := r.single(ctx, u, p, in.ItemID, dto.Series)
		if err != nil {
			return out, err
		}
		q.ParentID = show.Id
		if in.Mode == nativelibrary.Seasons {
			restrict("Season")
			options.order = []string{"s ASC"}
			if in.Special != nil {
				options.extra = func(w string, a []any) (string, []any) {
					if *in.Special {
						return w + " AND s=0", a
					}
					return w + " AND s>0", a
				}
			}
		} else {
			restrict("Episode")
			options.order = []string{"s ASC", "e ASC"}
			options.startID = in.StartItemID
			season := in.Season
			if in.SeasonID != "" {
				item, err := r.single(ctx, u, p, in.SeasonID, dto.Season)
				if err != nil {
					return out, err
				}
				if item.SeriesId != show.Id || item.IndexNumber == nil || season != nil && *season != *item.IndexNumber {
					return out, nativelibrary.ErrNotFound
				}
				season = item.IndexNumber
			}
			if season != nil {
				options.extra = func(w string, a []any) (string, []any) {
					a = append(a, *season)
					return w + fmt.Sprintf(" AND s=$%d::int", len(a)), a
				}
			}
		}
	case nativelibrary.Latest:
		q.Limit = min(q.Limit, 100)
		if q.ParentID != "" {
			if _, err := r.single(ctx, u, p, q.ParentID, dto.CollectionFolder); err != nil {
				return out, err
			}
		}
		grouped := in.GroupItems == nil || *in.GroupItems
		if len(q.IncludeItemTypes) == 0 {
			q.IncludeItemTypes = []string{"Movie", "Episode"}
		}
		if grouped {
			types := make([]string, len(q.IncludeItemTypes))
			for i, k := range q.IncludeItemTypes {
				types[i] = k
				if strings.EqualFold(k, "Episode") {
					types[i] = "Series"
				}
			}
			q.IncludeItemTypes = types
			q.IsFolder = nil
		}
		options.order = []string{"latest_created DESC"}
	case nativelibrary.Hints:
		term := strings.TrimSpace(q.SearchTerm)
		q.Limit = min(q.Limit, 100)
		if term == "" || !utf8.ValidString(term) {
			return out, nativelibrary.ErrInvalidQuery
		}
		if len(q.IncludeItemTypes) == 0 {
			q.IncludeItemTypes = []string{"Movie", "Series", "Episode"}
		}
		q.SearchTerm = ""
		options.search = term
		options.bounded = true
	case nativelibrary.Genres, nativelibrary.Filters:
		return r.facets(ctx, u, p, q)
	case nativelibrary.Similar:
		q.Limit = min(q.Limit, 20)
		item, err := r.single(ctx, u, p, in.ItemID, "")
		if err != nil {
			return out, err
		}
		if len(item.Genres) == 0 {
			return out, nil
		}
		restrict(string(item.Type))
		q.Genres = item.Genres
		q.SortBy = []string{"CommunityRating", "SortName"}
		q.SortOrder = []string{"Descending", "Ascending"}
		options.extra = func(w string, a []any) (string, []any) {
			a = append(a, item.Id)
			return w + fmt.Sprintf(" AND id<>$%d::text", len(a)), a
		}
	default:
		return out, nativelibrary.ErrInvalidQuery
	}
	items, total, err := r.query(ctx, u, p, q, options)
	if err != nil {
		return out, err
	}
	if in.Mode == nativelibrary.Episodes && (in.Season != nil || in.SeasonID != "") && total == 0 {
		return out, nativelibrary.ErrNotFound
	}
	out.Items, out.Total = items, total
	if in.Mode == nativelibrary.Hints {
		out.Hints = dto.SearchHintResult{SearchHints: dto.Array[dto.SearchHint]{}, TotalRecordCount: total}
		for _, item := range items {
			hint := dto.SearchHint{Id: item.Id, ItemId: item.Id, Name: item.Name, Type: item.Type, ProductionYear: item.ProductionYear, PrimaryImageTag: item.ImageTags["Primary"], Series: item.SeriesName, SeriesId: item.SeriesId}
			if hint.PrimaryImageTag == "" {
				hint.PrimaryImageTag = item.SeriesPrimaryImageTag
			}
			if len(item.BackdropImageTags) > 0 {
				hint.ThumbImageTag = item.BackdropImageTags[0]
			} else if len(item.ParentBackdropImageTags) > 0 {
				hint.ThumbImageTag = item.ParentBackdropImageTags[0]
			}
			out.Hints.SearchHints = append(out.Hints.SearchHints, hint)
		}
	}
	return out, nil
}
