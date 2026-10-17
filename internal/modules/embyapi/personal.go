package embyapi

import (
	"errors"
	"media-tool/internal/modules/nativelibrary"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) personal(w http.ResponseWriter, r *http.Request, info RequestContext) {
	service, ok := s.options.Browse.(nativelibrary.PersonalService)
	if !ok || info.User == nil || info.Permissions == nil {
		s.router.fail(w, 503)
		return
	}
	var patch *nativelibrary.PersonalPatch
	if r.Method != http.MethodGet {
		values, e := playbackParameters(r, info.Parameters)
		if e != nil {
			s.router.fail(w, 400)
			return
		}
		v, e := parsePersonal(values)
		if e != nil {
			s.router.fail(w, 400)
			return
		}
		path := strings.ToLower(r.URL.Path)
		if strings.Contains(path, "/playeditems/") {
			yes := r.Method != http.MethodDelete
			v = nativelibrary.PersonalPatch{Played: &yes, LastPlayedAt: v.LastPlayedAt}
			if !yes {
				v.LastPlayedAt = nil
			}
		}
		if strings.Contains(path, "/favoriteitems/") {
			yes := r.Method != http.MethodDelete
			v = nativelibrary.PersonalPatch{Favorite: &yes}
		}
		patch = &v
	}
	out, err := service.PersonalData(r.Context(), *info.User, info.Permissions, r.PathValue("itemId"), patch)
	if errors.Is(err, nativelibrary.ErrPersonalBatchTooLarge) {
		writeAuthError(w, 400, "单次最多标记5000集，请按季分批操作")
		return
	}
	if err != nil {
		s.browseError(w, err)
		return
	}
	s.write(w, out)
}
func parsePersonal(v Values) (nativelibrary.PersonalPatch, error) {
	var out nativelibrary.PersonalPatch
	var err error
	out.Played, err = v.Boolean("Played")
	if err != nil {
		return out, err
	}
	out.Favorite, err = v.Boolean("IsFavorite")
	if err != nil {
		return out, err
	}
	for _, spec := range []struct {
		key string
		max int64
	}{{"PlaybackPositionTicks", 1<<63 - 1}, {"PlayCount", 1000000000}} {
		text, e := v.Scalar(spec.key)
		if e != nil {
			return out, e
		}
		if text != "" {
			n, e := strconv.ParseInt(text, 10, 64)
			if e != nil || n < 0 || n > spec.max {
				return out, ErrInvalid
			}
			if spec.key == "PlayCount" {
				value := int(n)
				out.PlayCount = &value
			} else {
				out.PositionTicks = &n
			}
		}
	}
	for _, name := range []string{"DatePlayed", "LastPlayedDate"} {
		text, e := v.Scalar(name)
		if e != nil {
			return out, e
		}
		if text != "" {
			stamp, e := time.Parse(time.RFC3339Nano, text)
			if e != nil || stamp.Year() < 1 || stamp.Year() > 9999 {
				return out, ErrInvalid
			}
			stamp = stamp.UTC().Truncate(100 * time.Nanosecond)
			if out.LastPlayedAt != nil && !out.LastPlayedAt.Equal(stamp) {
				return out, ErrInvalid
			}
			out.LastPlayedAt = &stamp
		}
	}
	return out, nil
}
