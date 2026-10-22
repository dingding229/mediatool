package embyapi

import (
	"net/http"
	"strconv"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativelibrary"
)

func (s *Server) discover(w http.ResponseWriter, r *http.Request, info RequestContext, kind responseKind) {
	if s.options.Discovery == nil || info.User == nil || info.Permissions == nil {
		s.router.fail(w, 503)
		return
	}
	q, err := ParseQuery(info.Parameters)
	if err != nil {
		s.router.fail(w, 400)
		return
	}
	in := nativelibrary.DiscoveryQuery{Items: q.ItemQuery(), ItemID: r.PathValue("id")}
	switch kind {
	case resumeResponse:
		in.Mode = nativelibrary.Resume
		media, e := info.Parameters.list("MediaTypes", 8)
		if e != nil {
			s.router.fail(w, 400)
			return
		}
		in.MediaTypes = media

	case nextUpResponse:
		in.Mode = nativelibrary.NextUp

	case seasonsResponse:
		in.Mode = nativelibrary.Seasons
	case episodesResponse:
		in.Mode = nativelibrary.Episodes
	case latestResponse:
		in.Mode = nativelibrary.Latest
	case hintsResponse:
		in.Mode = nativelibrary.Hints
	case genresResponse:
		in.Mode = nativelibrary.Genres
	case filtersResponse:
		in.Mode = nativelibrary.Filters
	case similarResponse:
		in.Mode = nativelibrary.Similar
	}
	if kind == seasonsResponse || kind == episodesResponse {
		in.Items.SeriesID, in.Items.SeasonID = "", ""
		if q.SeriesID != "" && q.SeriesID != in.ItemID {
			s.router.fail(w, 400)
			return
		}
	}

	if kind == latestResponse || kind == hintsResponse || kind == similarResponse {
		if raw, err := info.Parameters.Scalar("Limit"); err != nil {
			s.router.fail(w, 400)
			return
		} else if raw == "" {
			in.Items.Limit = 20
		}
		maxLimit := 100
		if kind == similarResponse {
			maxLimit = 20
		}
		in.Items.Limit = min(in.Items.Limit, maxLimit)
	}
	if in.GroupItems, err = info.Parameters.Boolean("GroupItems"); err != nil {
		s.router.fail(w, 400)
		return
	}
	if in.Special, err = info.Parameters.Boolean("IsSpecialSeason"); err != nil {
		s.router.fail(w, 400)
		return
	}
	if _, err = info.Parameters.Boolean("IsMissing"); err != nil {
		s.router.fail(w, 400)
		return
	}
	if in.SeasonID, err = info.Parameters.Scalar("SeasonId"); err != nil || in.SeasonID != "" && !opaqueID(in.SeasonID) {
		s.router.fail(w, 400)
		return
	}
	if in.StartItemID, err = info.Parameters.Scalar("StartItemId"); err != nil || in.StartItemID != "" && !opaqueID(in.StartItemID) {
		s.router.fail(w, 400)
		return
	}
	if raw, err := info.Parameters.Scalar("Season"); err != nil {
		s.router.fail(w, 400)
		return
	} else if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 10000 {
			s.router.fail(w, 400)
			return
		}
		in.Season = &n
	}
	if kind == nextUpResponse {
		if in.ItemID, err = info.Parameters.Scalar("SeriesId"); err != nil || len(in.ItemID) > 64 {
			s.router.fail(w, 400)
			return
		}
		var chosen *bool
		for _, name := range []string{"EnableSpecials", "IncludeSpecials", "EnableSpecialEpisodes"} {
			v, e := info.Parameters.Boolean(name)
			if e != nil || chosen != nil && v != nil && *chosen != *v {
				s.router.fail(w, 400)
				return
			}
			if v != nil {
				chosen = v
			}
		}
		in.Special = chosen
	}
	result, err := s.options.Discovery.Discover(r.Context(), *info.User, info.Permissions, in)
	if err != nil {
		s.browseError(w, err)
		return
	}
	if err := s.sourcePaths(r, result.Items); err != nil {
		s.router.fail(w, 503)
		return
	}
	switch kind {
	case latestResponse:
		s.write(w, dto.Array[dto.BaseItemDto](result.Items))
	case hintsResponse:
		s.write(w, result.Hints)
	case genresResponse:
		s.write(w, dto.QueryResult[dto.NameIdPair]{Items: dto.Array[dto.NameIdPair](result.Genres), TotalRecordCount: result.Total})
	case filtersResponse:
		s.write(w, result.Filters)
	default:
		start := int(q.StartIndex)
		s.write(w, dto.QueryResult[dto.BaseItemDto]{Items: dto.Array[dto.BaseItemDto](result.Items), TotalRecordCount: result.Total, StartIndex: &start})
	}
}
