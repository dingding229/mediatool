package embyapi

import (
	"errors"
	"media-tool/internal/modules/nativelibrary"
	"net/http"

	"media-tool/internal/modules/embyapi/dto"
)

func (s *Server) browse(w http.ResponseWriter, r *http.Request, info RequestContext, kind responseKind) {
	if s.options.Browse == nil || info.User == nil || info.Permissions == nil {
		s.router.fail(w, 503)
		return
	}
	q, err := ParseQuery(info.Parameters)
	if err != nil {
		s.router.fail(w, 400)
		return
	}
	if kind == viewsResponse {
		items, err := s.options.Browse.Views(r.Context(), *info.User, info.Permissions)
		if err != nil {
			s.browseError(w, err)
			return
		}
		s.write(w, dto.QueryResult[dto.BaseItemDto]{Items: dto.Array[dto.BaseItemDto](items), TotalRecordCount: len(items)})
		return
	}
	if kind == countsResponse {
		counts, err := s.options.Browse.Counts(r.Context(), *info.User, info.Permissions, q.ItemQuery())
		if err != nil {
			s.browseError(w, err)
			return
		}
		s.write(w, counts)
		return
	}
	if kind == detailResponse {
		id := r.PathValue("itemId")
		if id == "" {
			id = r.PathValue("id")
		}
		q.IDs = []string{id}
		q.StartIndex = 0
		q.Limit = 1
		if len(info.Parameters.entries[foldKey("Fields")]) == 0 {
			q.Fields = []string{"Overview", "People"}
		}
	}
	items, total, err := s.options.Browse.Query(r.Context(), *info.User, info.Permissions, q.ItemQuery())
	if err != nil {
		s.browseError(w, err)
		return
	}
	if kind == detailResponse {
		if len(items) != 1 {
			s.router.fail(w, 404)
			return
		}
		s.write(w, items[0])
		return
	}
	start := int(q.StartIndex)
	s.write(w, dto.QueryResult[dto.BaseItemDto]{Items: dto.Array[dto.BaseItemDto](items), TotalRecordCount: total, StartIndex: &start})
}
func (s *Server) browseError(w http.ResponseWriter, err error) {
	// Only domain statuses leave this boundary, never wrapped driver/SQL text.
	status := DomainErrorStatus(err)
	if errors.Is(err, nativelibrary.ErrDiscoveryTooBroad) {
		status = 400
	}
	if status == 500 || status == 501 {
		status = 503
	}
	s.router.fail(w, status)
}
