package nativelibrary

import "media-tool/internal/modules/embyapi/dto"

// ApplyEffectiveText consumes the SQL-merged, page-bounded text. Do not merge a
// provider snapshot after this: an explicit empty/null overlay must stay empty.
func ApplyEffectiveText(item *dto.BaseItemDto, m StoredMovieMetadata) {
	item.OriginalTitle = m.OriginalTitle
	item.SortName = m.SortName
	if item.SortName == "" {
		item.SortName = item.Name
	}
	item.Overview = m.Overview
	item.Genres = append(dto.Array[string]{}, m.Genres...)
	item.Tags = append(dto.Array[string]{}, m.Tags...)
	item.Taglines = nil
	if m.Tagline != "" {
		item.Taglines = dto.Array[string]{m.Tagline}
	}
	item.CommunityRating = m.CommunityRating
	item.OfficialRating = m.OfficialRating
	item.PremiereDate = parseUTCDate(m.PremiereDate)
	if item.Type == dto.Season || item.Type == dto.Episode {
		item.SeriesName = m.SeriesName
	}
	if item.Type == dto.Episode {
		item.SeasonName = m.SeasonName
	}
}
