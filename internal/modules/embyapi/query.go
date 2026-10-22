package embyapi

import (
	"slices"
	"strconv"
	"strings"
)

// Query contains canonical allowlisted names, never a raw SQL/order fragment.
type Query struct {
	StartIndex                                                         int64
	Limit                                                              int
	SortBy, SortOrder, Fields, IDs, IncludeItemTypes, ExcludeItemTypes []string
	ParentID, SeriesID, SeasonID                                       string
	Genres, Years                                                      []string
	SearchTerm                                                         string
	Recursive, IsFolder, IsPlayed, IsFavorite, IsResumable             *bool
}

func ParseQuery(v Values) (Query, error) {
	q := Query{Limit: 100, SortBy: []string{"SortName"}, SortOrder: []string{"Ascending"}}
	start, err := v.Scalar("StartIndex")
	if err != nil {
		return Query{}, err
	}
	if start != "" {
		q.StartIndex, err = strconv.ParseInt(start, 10, 64)
		if err != nil || q.StartIndex < 0 || q.StartIndex > 1000000 {
			return Query{}, ErrInvalid
		}
	}
	limit, err := v.Scalar("Limit")
	if err != nil {
		return Query{}, err
	}
	if limit != "" {
		n, err := strconv.ParseInt(limit, 10, 64)
		if err != nil || n < 0 || n > 200 {
			return Query{}, ErrInvalid
		}
		q.Limit = int(n)
	}
	sorts := strings.Fields("Name SortName DateCreated DateLastContentAdded PremiereDate ProductionYear CommunityRating Random DatePlayed PlayCount Runtime IndexNumber ParentIndexNumber IsFolder OfficialRating")
	// Standard Emby type filters can include kinds this catalog does not store.
	// Preserve them as filters (zero matches), never reject the whole union or
	// drop them and accidentally turn an unsupported-only query into all items.
	types := strings.Fields("Movie Series Season Episode CollectionFolder Folder Video MusicVideo MusicAlbum")
	fields := strings.Fields("Overview Genres People ProviderIds MediaSources MediaStreams PrimaryImageAspectRatio DateCreated PremiereDate ProductionYear OriginalTitle SortName CommunityRating OfficialRating UserData ChildCount RecursiveItemCount ParentId SeriesName SeriesId SeasonId Container RunTimeTicks Taglines Studios")
	for _, spec := range []struct {
		key     string
		target  *[]string
		allowed []string
		ignore  bool
	}{
		{"SortBy", &q.SortBy, sorts, false}, {"SortOrder", &q.SortOrder, []string{"Ascending", "Descending"}, false},
		{"Fields", &q.Fields, fields, true}, {"IncludeItemTypes", &q.IncludeItemTypes, types, false}, {"ExcludeItemTypes", &q.ExcludeItemTypes, types, false},
	} {
		list, err := v.list(spec.key, 64)
		if err != nil {
			return Query{}, err
		}
		if len(list) == 0 {
			continue
		}
		selected := []string{}
		for _, item := range list {
			found := ""
			for _, allowed := range spec.allowed {
				if strings.EqualFold(item, allowed) {
					found = allowed
					break
				}
			}
			if found == "" {
				if spec.ignore {
					continue
				}
				return Query{}, ErrInvalid
			}
			if !slices.Contains(selected, found) {
				selected = append(selected, found)
			}
		}
		*spec.target = selected
	}
	if len(q.SortOrder) > 1 && len(q.SortOrder) != len(q.SortBy) {
		return Query{}, ErrInvalid
	}
	q.IDs, err = v.list("Ids", 200)
	if err != nil {
		return Query{}, err
	}
	for _, id := range q.IDs {
		if !opaqueID(id) {
			return Query{}, ErrInvalid
		}
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"SeriesId", &q.SeriesID}, {"SeasonId", &q.SeasonID}} {
		*f.v, err = v.Scalar(f.name)
		if err != nil || *f.v != "" && !opaqueID(*f.v) {
			return Query{}, ErrInvalid
		}
	}

	q.ParentID, err = v.Scalar("ParentId")
	if err != nil || (q.ParentID != "" && !opaqueID(q.ParentID)) {
		return Query{}, ErrInvalid
	}
	for _, spec := range []struct {
		key    string
		target **bool
	}{{"Recursive", &q.Recursive}, {"IsFolder", &q.IsFolder}, {"IsPlayed", &q.IsPlayed}, {"IsFavorite", &q.IsFavorite}, {"IsResumable", &q.IsResumable}} {
		*spec.target, err = v.Boolean(spec.key)
		if err != nil {
			return Query{}, err
		}
	}

	filters, e := v.list("Filters", 16)
	if e != nil {
		return Query{}, e
	}
	for _, f := range filters {
		var target **bool
		value := true
		switch strings.ToLower(f) {
		case "isresumable":
			target = &q.IsResumable
		case "isplayed":
			target = &q.IsPlayed
		case "isunplayed":
			target = &q.IsPlayed
			value = false
		case "isfavorite":
			target = &q.IsFavorite
		default:
			continue
		}
		if *target != nil && **target != value {
			return Query{}, ErrInvalid
		}
		v := value
		*target = &v
	}

	q.Genres, err = v.list("Genres", 64)
	if err != nil {
		return Query{}, err
	}
	q.Years, err = v.list("Years", 100)
	if err != nil {
		return Query{}, err
	}
	for _, year := range q.Years {
		n, e := strconv.Atoi(year)
		if e != nil || n < 1000 || n > 9999 {
			return Query{}, ErrInvalid
		}
	}
	q.SearchTerm, err = v.Scalar("SearchTerm")
	if err != nil || len(q.SearchTerm) > 256 {
		return Query{}, ErrInvalid
	}
	for _, name := range []string{"EnableImages", "EnableUserData"} {
		if _, e := v.Boolean(name); e != nil {
			return Query{}, e
		}
	}
	imageLimit, e := v.Scalar("ImageTypeLimit")
	if e != nil {
		return Query{}, e
	}
	if imageLimit != "" {
		n, e := strconv.Atoi(imageLimit)
		if e != nil || n < 0 || n > 100 {
			return Query{}, ErrInvalid
		}
	}
	return q, nil
}
func opaqueID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}
