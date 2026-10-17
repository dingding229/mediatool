package embyapi

import (
	"log/slog"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Route struct {
	Method, Pattern string
	Handler         http.HandlerFunc
}
type Options struct {
	Logger         *slog.Logger
	TrustedProxies []string
}
type compiledRoute struct {
	Route
	pattern     *regexp.Regexp
	names       []string
	specificity int
}

// Router is immutable after construction and safe for concurrent requests.
// CORS access is not granted here: no origin reflection or cookie credentials.
// T07 may apply an explicit management-configured origin policy outside it.
type Router struct {
	logMu     sync.Mutex
	logWindow int64
	logCount  int
	routes    []compiledRoute
	logger    *slog.Logger
	proxies   []string
}

func NewRouter(routes []Route, options Options) (*Router, error) {
	if len(routes) > 256 {
		return nil, ErrInvalid
	}
	if !clientinfo.ValidateProxies(options.TrustedProxies) {
		return nil, ErrInvalid
	}
	router := &Router{logger: options.Logger, proxies: slices.Clone(options.TrustedProxies)}
	if router.logger == nil {
		router.logger = slog.Default()
	}
	keys := map[string]bool{}
	for _, route := range routes {
		if route.Handler == nil || !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}, route.Method) {
			return nil, ErrInvalid
		}
		compiled, err := compileRoute(route)
		if err != nil {
			return nil, err
		}
		key := route.Method + compiled.pattern.String()
		if keys[key] {
			return nil, ErrInvalid
		}
		keys[key] = true
		router.routes = append(router.routes, compiled)
	}
	return router, nil
}
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w = &headWriter{ResponseWriter: w}
	}
	path, err := normalizePath(r.URL.Path)
	escaped := strings.ToLower(r.URL.EscapedPath())
	if err != nil || strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") {
		router.fail(w, 400)
		return
	}
	best, selected := -1, -1
	var captures []string
	methods := []string{http.MethodOptions}
	for i, candidate := range router.routes {
		parts := candidate.pattern.FindStringSubmatch(path)
		if parts == nil || candidate.specificity < best {
			continue
		}
		if candidate.specificity > best {
			best = candidate.specificity
			selected = -1
			methods = []string{http.MethodOptions}
		}
		methods = append(methods, candidate.Method)
		if candidate.Method == http.MethodGet {
			methods = append(methods, http.MethodHead)
		}
		if candidate.Method == r.Method || (r.Method == http.MethodHead && candidate.Method == http.MethodGet && selected < 0) {
			selected = i
			captures = parts[1:]
		}
	}
	if best < 0 {
		router.logUnknown(r)
		router.fail(w, 404)
		return
	}
	slices.Sort(methods)
	methods = slices.Compact(methods)
	w.Header().Set("Allow", strings.Join(methods, ", "))
	if r.Method == http.MethodOptions {
		// Static header allowlist only. No Access-Control-Allow-Origin/Credentials:
		// an unauthenticated preflight cannot grant access to an arbitrary origin.
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Emby-Authorization, X-Emby-Token, X-MediaBrowser-Token, Content-Type, Range")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if selected < 0 {
		router.fail(w, 405)
		return
	}
	info, err := NewRequestContext(r, router.proxies)
	if err != nil {
		router.fail(w, 400)
		return
	}
	copy := r.Clone(r.Context())
	if r.Method == http.MethodHead && router.routes[selected].Method == http.MethodGet {
		copy.Method = http.MethodGet
	}
	copy.URL.Path = path
	copy.URL.RawPath = ""
	copy = WithRequestInfo(copy, info)
	for i, name := range router.routes[selected].names {
		copy.SetPathValue(name, captures[i])
	}
	router.routes[selected].Handler(w, copy)
}
func (router *Router) fail(w http.ResponseWriter, status int) {
	if err := WriteError(w, status); err != nil {
		router.logger.Debug("native Emby error response interrupted", "status", status)
	}
}
func normalizePath(raw string) (string, error) {
	if len(raw) > 4096 || !validText(raw, 4096) || strings.Contains(raw, "\\") {
		return "", ErrInvalid
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "." || segment == ".." {
			return "", ErrInvalid
		}
	}
	clean := path.Clean("/" + raw)
	for _, prefix := range []string{"/emby", "/mediabrowser"} {
		if strings.EqualFold(clean, prefix) {
			return "/", nil
		}
		if len(clean) > len(prefix) && strings.EqualFold(clean[:len(prefix)], prefix) && clean[len(prefix)] == '/' {
			return clean[len(prefix):], nil
		}
	}
	return clean, nil
}

var parameterName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

func compileRoute(route Route) (compiledRoute, error) {
	out := compiledRoute{Route: route}
	normalized, err := normalizePath(route.Pattern)
	if err != nil || strings.ContainsAny(normalized, "?#") {
		return out, ErrInvalid
	}
	segments := strings.Split(strings.TrimPrefix(normalized, "/"), "/")
	pattern := make([]string, 0, len(segments))
	for i, segment := range segments {
		if segment == "*" {
			if i != len(segments)-1 || slices.Contains(out.names, "wildcard") {
				return out, ErrInvalid
			}
			out.names = append(out.names, "wildcard")
			pattern = append(pattern, "(.+)")
			continue
		}
		prefix, rest, hasParameter := strings.Cut(segment, "{")
		if hasParameter {
			if !strings.HasSuffix(rest, "}") {
				return out, ErrInvalid
			}
			name := strings.TrimSuffix(rest, "}")
			if !parameterName.MatchString(name) || slices.Contains(out.names, name) || strings.ContainsAny(prefix, "}*") {
				return out, ErrInvalid
			}
			if prefix != "" && !strings.HasSuffix(prefix, ".") {
				return out, ErrInvalid
			}
			out.names = append(out.names, name)
			pattern = append(pattern, regexp.QuoteMeta(strings.ToLower(prefix))+"([^/]+)")
			if prefix != "" {
				out.specificity++
			}
		} else {
			if strings.ContainsAny(segment, "}*") {
				return out, ErrInvalid
			}
			out.specificity++
			pattern = append(pattern, regexp.QuoteMeta(strings.ToLower(segment)))
		}
	}
	out.pattern, err = regexp.Compile("(?i)^/" + strings.Join(pattern, "/") + "$")
	return out, err
}

type headWriter struct {
	http.ResponseWriter
	started bool
}

func (w *headWriter) WriteHeader(status int) {
	if !w.started {
		w.started = true
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *headWriter) Write(body []byte) (int, error) {
	if !w.started {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(body))
		}
		w.WriteHeader(200)
	}
	return len(body), nil
}
