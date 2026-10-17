package embyapi

import (
	"media-tool/internal/modules/embyapi/dto"
	"net/http"
	"regexp"
	"strings"
)

var preferencesID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,36}$`)

func (s *Server) startup(w http.ResponseWriter, r *http.Request, info RequestContext, kind responseKind) {
	switch kind {
	case endpointResponse:
		// Conservative defaults, not an assertion that a proxy's private IP is LAN.
		s.write(w, dto.EndpointInfo{})
	case brandingResponse:
		s.write(w, dto.BrandingConfiguration{})
	case brandingCSSResponse:
		// An empty local stylesheet is deliberate: no arbitrary external resources.
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.WriteHeader(200)
	case displayResponse:
		id := r.PathValue("id")
		if id == "" {
			id = "usersettings"
		}
		if !preferencesID.MatchString(id) {
			s.router.fail(w, 400)
			return
		}
		s.write(w, dto.DefaultDisplayPreferences(id))
	case localizationResponse:
		// Read-only legal empty catalogues; not QueryResult, and never echo a path.
		switch strings.ToLower(r.PathValue("wildcard")) {
		case "countries", "cultures", "parentalratings", "options":
			s.write(w, []struct{}{})
		default:
			s.router.logUnknown(r)
			s.router.fail(w, 404)
		}
	case startupSessionsResponse:
		s.write(w, []dto.SessionInfo{})
	case userConfigurationResponse:
		s.write(w, dto.UserConfiguration{SubtitleMode: "Default"})
	case socketResponse:
		// Never upgrade/hijack: REST discovery, progress and heartbeat remain usable.
		writeAuthError(w, 501, "不支持 WebSocket，请使用 REST 查询和播放上报")
	}
}
