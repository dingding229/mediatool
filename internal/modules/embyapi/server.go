package embyapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServerOptions struct {
	Playback      nativeplayback.Service
	RequestOrigin func(*http.Request) (string, error)
	Settings      nativelibrary.ServerSettingsReader
	PublicURL     string
	Logger        *slog.Logger
	Images        ImageResolver
	Auth          contracts.AccountService
	Browse        nativelibrary.BrowseService
	Discovery     nativelibrary.DiscoveryService
	Permissions   func() contracts.PermissionChecker
	ClientAddress func(*http.Request) (clientinfo.Address, error)
}
type serverSettingsKey struct{}
type Server struct {
	router          *Router
	options         ServerOptions
	fallbackWarning sync.Once
	startupLimits   imageLimiter
	requests        chan struct{}
	browsePaths     map[string]bool
}

func NewServer(options ServerOptions) (*Server, error) {
	if options.Settings == nil {
		return nil, ErrInvalid
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.PublicURL != "" && !validOrigin(options.PublicURL) {
		return nil, ErrInvalid
	}
	s := &Server{options: options, browsePaths: map[string]bool{}, requests: make(chan struct{}, 64)}
	routes := []Route{}
	for _, e := range nativeEndpoints() {
		if e.kind >= viewsResponse && e.kind <= similarResponse || e.kind == personalResponse || e.kind == resumeResponse || e.kind == nextUpResponse {
			s.browsePaths[e.pattern] = true
		}
		routes = append(routes, Route{e.method, e.pattern, s.handle(e)})
	}
	router, err := NewRouter(routes, Options{Logger: options.Logger})
	if err != nil {
		return nil, err
	}
	s.router = router
	return s, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w = &headWriter{ResponseWriter: w}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	normalized, err := normalizePath(r.URL.Path)
	if err != nil || strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%2f") || strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%5c") {
		s.router.ServeHTTP(w, r)
		return
	}
	known, selected, best := false, "", -1
	for _, route := range s.router.routes {
		if route.pattern.MatchString(normalized) && route.specificity > best {
			known, selected, best = true, route.Pattern, route.specificity
		}
	}
	// Unknown/admin/PWA paths stay 404 even when disabled or the database is down.
	if !known {
		s.router.ServeHTTP(w, r)
		return
	}
	// Reject overload before permission/DB/provider work. No global lock spans
	// processing, and queued work cannot grow without bound inside the handler.
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		w.Header().Set("Retry-After", "1")
		s.router.fail(w, 503)
		return
	}
	// Bound even compatibility/no-content endpoints that do not decode JSON.
	if r.ContentLength > 1<<20 {
		s.router.fail(w, http.StatusRequestEntityTooLarge)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	}
	if selected == "/Items/{id}/PlaybackInfo" || strings.HasPrefix(selected, "/Sessions/Playing") || strings.Contains(selected, "/PlayingItems/") || strings.Contains(selected, "/subtitles/") || strings.Contains(selected, "/Subtitles/") {
		s.requestBudget(w, r, BrowseRequestBudget, func(w http.ResponseWriter, r *http.Request) { s.serveKnown(w, r, normalized) })
		return
	}
	if selected == "/Videos/{id}/stream" || selected == "/Videos/{id}/stream.{ext}" || selected == "/Videos/{id}/original.{ext}" || selected == "/Items/{id}/Download" {
		s.requestBudget(w, r, 28*time.Second, func(w http.ResponseWriter, r *http.Request) { s.serveKnown(w, r, normalized) })
		return
	}
	if s.browsePaths[selected] {
		s.browseBudget(w, r, func(w http.ResponseWriter, r *http.Request) { s.serveKnown(w, r, normalized) })
		return
	}
	s.serveKnown(w, r, normalized)
}
func (s *Server) serveKnown(w http.ResponseWriter, r *http.Request, normalized string) {
	value, err := s.options.Settings.Get(r.Context())
	if err != nil {
		s.router.fail(w, 503)
		return
	}
	if !value.Enabled && !strings.EqualFold(normalized, "/System/Info/Public") && !strings.EqualFold(normalized, "/System/Ping") {
		s.router.fail(w, 503)
		return
	}
	s.router.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverSettingsKey{}, value)))
}
func (s *Server) handle(e endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, ok := RequestInfo(r.Context())
		if !ok {
			s.router.fail(w, 400)
			return
		}
		if s.options.ClientAddress != nil {
			address, err := s.options.ClientAddress(r)
			if err != nil {
				s.router.fail(w, 503)
				return
			}
			info.Address = address
		}
		if e.public && e.kind != imageResponse && e.kind != authenticateResponse && !s.startupLimits.allow(info.Address.ClientIP, time.Now()) {
			w.Header().Set("Retry-After", "60")
			s.router.fail(w, 429)
			return
		}
		if !e.public {
			if s.options.Auth != nil && info.Auth.Token != "" {
				user, device, token, err := s.options.Auth.Authenticate(r.Context(), info.Auth.Token, deviceInfo(info))
				if err == nil {
					info.User, info.Device, info.Token = &user, &device, &token
				} else if errors.Is(err, contracts.ErrUnavailable) {
					s.router.fail(w, 503)
					return
				}
			}
			if !e.public && info.User == nil {
				s.router.fail(w, 401)
				return
			}
		}
		if info.User != nil {
			if userID := r.PathValue("userId"); userID != "" && canonicalUserID(userID) != info.User.ID {
				s.router.fail(w, 403)
				return
			}
			if info.Auth.UserID != "" && canonicalUserID(info.Auth.UserID) != info.User.ID {
				s.router.fail(w, 403)
				return
			}
			s.options.Logger.Debug("Native token verified", "tokenDigestPrefix", fmt.Sprintf("%x", info.Token.Digest[:6]))
		}
		if info.User != nil {
			info.Permissions = contracts.NewPermissionChecker()
			if s.options.Permissions != nil {
				if scoped := s.options.Permissions(); scoped != nil {
					info.Permissions = scoped
				}
			}
		}
		r = WithRequestInfo(r, info)
		value, ok := r.Context().Value(serverSettingsKey{}).(nativelibrary.ServerSettings)
		if !ok {
			s.router.fail(w, 503)
			return
		}
		switch e.kind {
		case endpointResponse, brandingResponse, brandingCSSResponse, displayResponse, localizationResponse, socketResponse, startupSessionsResponse, userConfigurationResponse:
			s.startup(w, r, info, e.kind)
		case personalResponse:
			s.personal(w, r, info)
		case reportResponse:
			s.report(w, r, info)
		case subtitleResponse:
			s.playback(w, r, info, true)
		case playbackResponse, streamResponse:
			s.playback(w, r, info, e.kind == streamResponse)
		case seasonsResponse, episodesResponse, latestResponse, hintsResponse, genresResponse, filtersResponse, similarResponse, resumeResponse, nextUpResponse:
			s.discover(w, r, info, e.kind)
		case viewsResponse, browseResponse, detailResponse, countsResponse:
			s.browse(w, r, info, e.kind)
		case publicInfoResponse, systemInfoResponse:
			origin, err := s.publicOrigin(r, value)
			if err != nil {
				s.router.fail(w, 503)
				return
			}
			info := dto.PublicSystemInfo{Id: value.ServerID, ServerName: value.ServerName, Version: value.Version, ProductName: "Emby Server", LocalAddress: origin, StartupWizardCompleted: true}
			if e.kind == systemInfoResponse {
				s.write(w, dto.SystemInfo{PublicSystemInfo: info})
			} else {
				s.write(w, info)
			}
		case authenticateResponse:
			s.authLogin(w, r, info, value)
		case userResponse:
			s.authUser(w, r, info, value)
		case passwordResponse:
			s.authPassword(w, r, info)
		case logoutResponse:
			s.authLogout(w, r, info)
		case publicUsersResponse:
			if !value.PublicUserList {
				s.write(w, []contracts.PublicUser{})
				return
			}
			s.authPublicUsers(w, r, value)
		case noContentResponse:
			w.WriteHeader(http.StatusNoContent)
		case imageResponse:
			s.image(w, r)
		case pingResponse:
			s.write(w, value.ServerName)
		default:
			s.empty(w, e.kind)
		}
	}
}

// Used only after the route's authentication gate; future tasks replace these.
func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	info, _ := RequestInfo(r.Context())
	if info.User == nil {
		s.router.fail(w, http.StatusUnauthorized)
		return
	}
	if s.options.Images == nil {
		s.router.fail(w, http.StatusNotFound)
		return
	}
	index := 0
	if raw := r.PathValue("index"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 100 {
			s.router.fail(w, http.StatusNotFound)
			return
		}
		index = n
	}
	request := ImageRequest{User: info.User, Permissions: info.Permissions, ItemID: r.PathValue("id"), Type: r.PathValue("type"), Index: index, Tag: queryValue(r, "tag"), Format: queryValue(r, "format"), RequestedWidth: queryWidth(r)}
	result, err := s.options.Images.ResolveImage(r.Context(), request)
	if errors.Is(err, ErrImageNotFound) {
		s.router.fail(w, http.StatusNotFound)
		return
	}
	if err != nil {
		s.router.fail(w, http.StatusServiceUnavailable)
		return
	}

	if result.ContentType == "" || result.Tag == "" || len(result.Body) == 0 {
		s.router.fail(w, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	if request.Tag == result.Tag {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	w.Header().Set("Vary", "X-Emby-Token, X-Emby-Authorization, Authorization")
	w.Header().Set("ETag", `"`+result.Tag+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == `"`+result.Tag+`"` || strings.TrimSpace(r.Header.Get("If-None-Match")) == result.Tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if _, err := w.Write(result.Body); err != nil {
		s.options.Logger.Debug("Native image response interrupted", "error", err)
	}
}
func queryValue(r *http.Request, name string) string {
	for key, values := range r.URL.Query() {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
func queryWidth(r *http.Request) int {
	best := 0
	for _, name := range []string{"maxWidth", "width", "fillWidth"} {
		if raw := queryValue(r, name); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > best && n <= 8192 {
				best = n
			}
		}
	}
	return best
}
func (s *Server) empty(w http.ResponseWriter, kind responseKind) {
	switch kind {
	case listResponse:
		s.write(w, dto.QueryResult[dto.BaseItemDto]{Items: dto.Array[dto.BaseItemDto]{}, TotalRecordCount: 0})
	case singleResponse:
		s.router.fail(w, 404)
	default:
		s.router.fail(w, 501)
	}
}
func (s *Server) write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.options.Logger.Debug("Native media response interrupted", "module", "native_media")
	}
}
func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 512 && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.Opaque == "" && raw == u.Scheme+"://"+u.Host
}
