package embyapi

import (
	"media-tool/internal/modules/nativeplayback"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) report(w http.ResponseWriter, r *http.Request, info RequestContext) {
	reporter, ok := s.options.Playback.(nativeplayback.Reporter)
	if !ok || info.User == nil || info.Device == nil || info.Permissions == nil {
		s.router.fail(w, 503)
		return
	}
	values, err := playbackParameters(r, info.Parameters)
	if err != nil {
		s.router.fail(w, 400)
		return
	}
	source, user, err := playbackQuery(values)
	if err != nil {
		s.router.fail(w, 400)
		return
	}
	if user != "" && canonicalUserID(user) != info.User.ID {
		s.router.fail(w, 403)
		return
	}
	q := nativeplayback.Report{Kind: nativeplayback.ReportPlaying, MediaSourceID: source}
	q.ItemID, err = values.Scalar("ItemId")
	if err != nil || len(q.ItemID) > 19 {
		s.router.fail(w, 400)
		return
	}
	if id := r.PathValue("itemId"); id != "" {
		if q.ItemID != "" && q.ItemID != id {
			s.router.fail(w, 400)
			return
		}
		q.ItemID = id
	}
	q.PlaySessionID, err = values.Scalar("PlaySessionId")
	if err != nil || len(q.PlaySessionID) > 64 {
		s.router.fail(w, 400)
		return
	}
	position, e := values.Scalar("PositionTicks")
	if e != nil {
		s.router.fail(w, 400)
		return
	}
	if position != "" {
		n, e := strconv.ParseInt(position, 10, 64)
		if e != nil || n < 0 {
			s.router.fail(w, 400)
			return
		}
		q.PositionTicks = &n
	}
	q.IsPaused, err = values.Boolean("IsPaused")
	if err != nil {
		s.router.fail(w, 400)
		return
	}
	event, err := values.Scalar("EventName")
	if err != nil || len(event) > 64 {
		s.router.fail(w, 400)
		return
	}
	if strings.EqualFold(event, "Pause") || strings.EqualFold(event, "Unpause") {
		paused := strings.EqualFold(event, "Pause")
		if q.IsPaused != nil && *q.IsPaused != paused {
			s.router.fail(w, 400)
			return
		}
		q.IsPaused = &paused
	}
	path := strings.ToLower(r.URL.Path)
	switch {
	case strings.HasSuffix(path, "/progress"):
		q.Kind = nativeplayback.ReportProgress
	case strings.HasSuffix(path, "/stopped") || r.Method == http.MethodDelete || strings.HasSuffix(path, "/delete"):
		q.Kind = nativeplayback.ReportStopped
	case strings.HasSuffix(path, "/ping"):
		q.Kind = nativeplayback.ReportPing
	}
	actor := nativeplayback.Actor{User: *info.User, Device: *info.Device, Permissions: info.Permissions, UserAgent: info.UserAgent, Method: info.Method, Address: info.Address}
	if err = reporter.Report(r.Context(), actor, q); err != nil {
		s.playbackError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
