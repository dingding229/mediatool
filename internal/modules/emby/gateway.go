package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/auth"
)

type GatewayOptions struct {
	Upstream                      *url.URL
	PublicOrigin                  *url.URL
	Enable302                     bool
	DirectPlayOnly                bool
	LegacyEnabled                 bool
	OwnedAsset                    func(context.Context, string) bool
	Builder                       strm.Builder
	STRMOutputDir, STRMPathPrefix string
	ResolverOrigins               []string
	Playback                      *playback.Engine
	ManagedURL                    func(context.Context, string) (storage.FileRef, error)
	Client                        *http.Client
	Identities                    IdentityStore
}
type Gateway struct {
	options GatewayOptions
	proxy   http.Handler
	client  *http.Client
}

var itemIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var originalPathPattern = regexp.MustCompile(`(?i)^/Videos/([A-Za-z0-9_-]{1,128})/original\.([A-Za-z0-9]+)$`)
var staticPathPattern = regexp.MustCompile(`(?i)^/Videos/([A-Za-z0-9_-]{1,128})/stream(?:\.([A-Za-z0-9]+))?$`)

func NewGateway(options GatewayOptions) *Gateway {
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if options.Identities == nil {
		options.Identities = NewMemoryIdentityStore()
	}
	g := &Gateway{options: options, client: &copyClient}
	proxy := newProxy(options.Upstream, g.MetadataLink, g.observeSession, g.MetadataHeaders, options.PublicOrigin)
	g.proxy = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), directPlayOnlyKey{}, options.DirectPlayOnly)))
	})
	return g
}
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rawPath := gatewayRequestPath(r.URL.Path)
	match := staticPathPattern.FindStringSubmatch(rawPath)
	original := false
	if match == nil {
		match = originalPathPattern.FindStringSubmatch(rawPath)
		original = match != nil
	}
	// Download/File aliases are safe only after the same caller authorization,
	// source mapping and file-grant checks as an original/static request.
	itemFile := directOnlyItemFilePath.FindStringSubmatch(rawPath)
	if g.options.DirectPlayOnly && itemFile != nil {
		match = []string{itemFile[0], itemFile[1], ""}
		original = true
	}
	if g.options.DirectPlayOnly {
		w.Header().Set("X-Media-Playback-Policy", "direct-only")
		if directOnlyOpaqueMediaPath.MatchString(rawPath) {
			gatewayError(w, 409, "direct_play_required", "仅直链播放：此媒体入口无法映射受管原始文件，禁止服务器中转")
			return
		}
		video := directOnlyMediaPath.FindStringSubmatch(rawPath)
		if (video != nil && !directOnlyMetadataPath.MatchString(rawPath)) || itemFile != nil {
			id := ""
			if video != nil {
				id = video[1]
			} else {
				id = itemFile[1]
			}
			if !g.options.Enable302 || g.options.Playback == nil || (r.Method != "GET" && r.Method != "HEAD") || match == nil || strings.EqualFold(match[2], "m3u8") {
				g.rejectTranscoding(w, r, id)
				return
			}
		}
	}
	if !g.options.Enable302 || g.options.Playback == nil || (r.Method != "GET" && r.Method != "HEAD") || match == nil || strings.EqualFold(match[2], "m3u8") {
		g.proxy.ServeHTTP(w, r)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		gatewayError(w, 400, "invalid_query", "播放参数无效")
		return
	}
	seen := map[string]bool{}
	for key, values := range query {
		lower := strings.ToLower(key)
		if seen[lower] {
			gatewayError(w, 400, "invalid_query", "播放参数大小写重复")
			return
		}
		seen[lower] = true
		if len(values) != 1 {
			gatewayError(w, 400, "invalid_query", "播放参数不能重复")
			return
		}
		if requiresTranscoding(key, values[0]) {
			g.fallback(w, r, "transcoding_parameters")
			return
		}
	}
	static := queryFold(query, "Static")
	if (!original && !strings.EqualFold(static, "true")) || (original && static != "" && !strings.EqualFold(static, "true")) {
		g.fallback(w, r, "non_static")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	_, item, err := g.authorizeItem(ctx, r, match[1])
	cancel()
	if err != nil {
		var upstream *upstreamError
		if errors.As(err, &upstream) && (upstream.Status == 401 || upstream.Status == 403 || upstream.Status == 404) {
			gatewayError(w, upstream.Status, "emby_access_denied", "Emby 拒绝当前用户读取此媒体")
			return
		}
		// Unknown authorization state is not a permission to issue a cloud redirect.
		g.fallback(w, r, "identity_or_permission_unconfirmed")
		return
	}
	source, ok := selectSource(item, queryFold(query, "MediaSourceId"))
	if !ok {
		g.fallback(w, r, "source_selection")
		return
	}
	ref, err := g.reference(r.Context(), source.Path)
	if err != nil && len(item.MediaSources) <= 1 && strings.HasSuffix(strings.ToLower(item.Path), ".strm") {
		ref, err = g.reference(r.Context(), item.Path)
	}
	if err != nil {
		g.fallback(w, r, "unmanaged_source")
		return
	}
	if match[2] != "" && !sameContainer(match[2], strings.TrimPrefix(path.Ext(ref.Path), ".")) {
		g.fallback(w, r, "container_mismatch")
		return
	}
	if container := queryFold(query, "Container"); container != "" && !sameContainer(container, strings.TrimPrefix(path.Ext(ref.Path), ".")) {
		g.fallback(w, r, "container_mismatch")
		return
	}
	w.Header().Set("X-Media-Gateway-Reason", "authorized_static")
	w.Header().Set("X-Media-Gateway", "emby-static")
	if g.options.DirectPlayOnly {
		g.options.Playback.ServeRedirect(w, r, ref)
	} else {
		g.options.Playback.Serve(w, r, ref)
	}
}
func requiresTranscoding(key, value string) bool {
	if value == "" {
		return false
	}
	switch strings.ToLower(key) {
	case "audiocodec", "videocodec", "audiobitrate", "videobitrate", "audiosamplerate", "audiochannels", "maxaudiochannels", "width", "height", "maxwidth", "maxheight", "maxvideobitdepth", "audiostreamindex", "videostreamindex":
		return true
	case "starttimeticks":
		return value != "0"
	case "subtitlemethod":
		return strings.EqualFold(value, "Encode") || strings.EqualFold(value, "BurnIn")
	}
	return false
}

type userIdentity struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Policy struct {
		EnableMediaPlayback *bool `json:"EnableMediaPlayback"`
	} `json:"Policy"`
}
type sourceInfo struct {
	ID   string `json:"Id"`
	Path string `json:"Path"`
}
type itemInfo struct {
	ID           string       `json:"Id"`
	Path         string       `json:"Path"`
	PlayAccess   string       `json:"PlayAccess"`
	MediaSources []sourceInfo `json:"MediaSources"`
}
type upstreamError struct{ Status int }

func (e *upstreamError) Error() string { return "Emby rejected diagnostic request" }
func (g *Gateway) authorizeItem(ctx context.Context, r *http.Request, itemID string) (userIdentity, itemInfo, error) {
	var user userIdentity
	var item itemInfo
	if !itemIDPattern.MatchString(itemID) {
		return user, item, errors.New("invalid item ID")
	}
	user, err := g.callerUser(ctx, r)
	if err != nil {
		return user, item, err
	}
	if user.Policy.EnableMediaPlayback == nil || !*user.Policy.EnableMediaPlayback {
		return user, item, &upstreamError{Status: http.StatusForbidden}
	}
	fields := url.Values{"Fields": {"MediaSources,Path"}}
	if err := g.get(ctx, r, "/Users/"+user.ID+"/Items/"+itemID, fields, &item); err != nil {
		return user, item, err
	}
	if item.ID != itemID {
		return user, item, errors.New("Emby item identity mismatch")
	}
	// Emby 4.10 may omit PlayAccess. Only accept the omission after a
	// caller-authenticated PlaybackInfo check for the selected actual source.
	if item.PlayAccess != "" && item.PlayAccess != "Full" {
		return user, item, &upstreamError{Status: http.StatusForbidden}
	}
	if item.PlayAccess == "" {
		var info struct {
			ErrorCode    string       `json:"ErrorCode"`
			MediaSources []sourceInfo `json:"MediaSources"`
		}
		if err := g.get(ctx, r, "/Items/"+itemID+"/PlaybackInfo", url.Values{"UserId": {user.ID}}, &info); err != nil {
			return user, item, err
		}
		selected, ok := selectSource(item, queryFold(r.URL.Query(), "MediaSourceId"))
		if info.ErrorCode != "" {
			return user, item, &upstreamError{Status: http.StatusForbidden}
		}
		authorized := false
		for _, source := range info.MediaSources {
			if ok && source.ID == selected.ID && source.Path == selected.Path && source.Path != "" {
				authorized = true
			}
		}
		if !authorized {
			return user, item, errors.New("Emby playback source unconfirmed")
		}
	}
	return user, item, nil
}

// Only caller-authenticated upstream profile data may attribute playback. Cached
// token->ID bindings are revalidated; query UserId and UA/IP are never identities.
func (g *Gateway) callerUser(ctx context.Context, r *http.Request) (userIdentity, error) {
	var user userIdentity
	token, credentialErr := unambiguousToken(r)
	if credentialErr != nil {
		return user, &upstreamError{Status: http.StatusUnauthorized}
	}
	if token == "" {
		return user, &upstreamError{Status: http.StatusUnauthorized}
	}
	key := IdentityKey(g.options.Upstream, token)
	id, lookupErr := g.options.Identities.Lookup(ctx, key)
	if lookupErr == nil && itemIDPattern.MatchString(id) {
		if err := g.get(ctx, r, "/Users/"+id, nil, &user); err != nil {
			var denied *upstreamError
			if errors.As(err, &denied) && denied.Status == 401 {
				_ = g.options.Identities.Forget(ctx, key)
			}
			return user, err
		}
		if user.ID != id {
			return user, ErrIdentityUnknown
		}
	} else {
		// Supported by some compatible servers. Never infer identity from a client
		// UserId, DeviceId, public-user list or an administrative API key.
		if err := g.get(ctx, r, "/Users/Me", nil, &user); err != nil {
			return user, err
		}
		if !itemIDPattern.MatchString(user.ID) {
			return user, ErrIdentityUnknown
		}
		_ = g.options.Identities.Remember(ctx, key, user.ID)
	}
	return user, nil
}
func (g *Gateway) VerifiedCaller(ctx context.Context, r *http.Request) (string, string, error) {
	u, err := g.callerUser(ctx, r)
	if err != nil {
		return "", "", err
	}
	return u.ID, u.Name, nil
}
func (g *Gateway) get(ctx context.Context, in *http.Request, endpoint string, extra url.Values, out any) error {
	target := *g.options.Upstream
	target.Path = strings.TrimRight(target.Path, "/") + endpoint
	target.RawPath = ""
	if extra == nil {
		extra = url.Values{}
	}
	// A single caller token is sent. Do not forward a spoofed UserId or
	// conflicting authentication header/cookie into the diagnostic request.
	target.RawQuery = extra.Encode()
	request, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		return errors.New("invalid Emby URL")
	}
	token, err := unambiguousToken(in)
	if err != nil || token == "" {
		return &upstreamError{Status: http.StatusUnauthorized}
	}
	request.Header.Set("X-Emby-Token", token)
	response, err := g.client.Do(request)
	if err != nil {
		return errors.New("Emby diagnostic request failed")
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if response.StatusCode != 200 {
		return &upstreamError{Status: response.StatusCode}
	}
	limited := io.LimitReader(response.Body, 1<<20)
	return json.NewDecoder(limited).Decode(out)
}
func selectSource(item itemInfo, id string) (sourceInfo, bool) {
	if id != "" {
		for _, source := range item.MediaSources {
			if source.ID == id {
				return source, true
			}
		}
		return sourceInfo{}, false
	}
	if len(item.MediaSources) == 1 {
		return item.MediaSources[0], true
	}
	if len(item.MediaSources) == 0 && strings.HasSuffix(strings.ToLower(item.Path), ".strm") {
		return sourceInfo{Path: item.Path}, true
	}
	return sourceInfo{}, false // do not choose the first of multiple media sources
}
func (g *Gateway) reference(ctx context.Context, raw string) (storage.FileRef, error) {
	if strings.HasSuffix(strings.ToLower(raw), ".strm") && g.options.STRMPathPrefix != "" {
		prefix := path.Clean(g.options.STRMPathPrefix)
		if !strings.HasPrefix(raw, prefix+"/") || path.Clean(raw) != raw {
			return storage.FileRef{}, errors.New("unmapped STRM path")
		}
		relative := strings.TrimPrefix(raw, prefix+"/")
		root, err := os.OpenRoot(g.options.STRMOutputDir)
		if err != nil {
			return storage.FileRef{}, errors.New("STRM output unavailable")
		}
		defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
		file, err := root.Open(relative)
		if err != nil {
			return storage.FileRef{}, errors.New("STRM file unavailable")
		}
		defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
		data, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil || len(data) > 8192 {
			return storage.FileRef{}, errors.New("STRM exceeds size limit")
		}
		raw = strings.TrimSpace(string(data))
	}
	return strm.ParseStable(ctx, raw, strm.ParseOptions{Builder: g.options.Builder, Origins: g.options.ResolverOrigins, LegacyEnabled: g.options.LegacyEnabled, ManagedURL: g.options.ManagedURL})
}

// CheckItem reports metadata authorization and mapping only. It never claims
// that the real Emby client decoded video or that media bytes were read.
type CheckResult struct {
	Authenticated     bool `json:"authenticated"`
	CanPlay           bool `json:"canPlay"`
	SourceMapped      bool `json:"sourceMapped"`
	MediaBytesChecked bool `json:"mediaBytesChecked"`
}

func (g *Gateway) CheckItem(ctx context.Context, r *http.Request, itemID, sourceID string) (CheckResult, error) {
	check := r.Clone(ctx)
	u := *r.URL
	q := u.Query()
	q.Set("MediaSourceId", sourceID)
	u.RawQuery = q.Encode()
	check.URL = &u
	user, item, err := g.authorizeItem(ctx, check, itemID)
	if err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{Authenticated: true, CanPlay: user.Policy.EnableMediaPlayback != nil && *user.Policy.EnableMediaPlayback}
	if source, ok := selectSource(item, sourceID); ok {
		_, err := g.reference(ctx, source.Path)
		result.SourceMapped = err == nil
	}
	return result, nil
}
func gatewayError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if checkedErr := json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}}); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/modules/emby/gateway.go:gatewayError")
		return
	}
}

func queryFold(q url.Values, key string) string {
	for k, v := range q {
		if strings.EqualFold(k, key) && len(v) == 1 {
			return v[0]
		}
	}
	return ""
}

func (g *Gateway) RedirectEnabled() bool { return g.options.Enable302 && g.options.Playback != nil }
func (g *Gateway) ManagedLink(ctx context.Context, raw string) bool {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Path != strm.Endpoint {
		return false
	}
	_, err = g.reference(ctx, raw)
	return err == nil
}

// Metadata redaction remains effective for revoked/rotated grants. Playback
// still uses reference(), which enforces current authorization separately.
func (g *Gateway) MetadataLink(ctx context.Context, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.RawPath != "" || u.Path != strm.Endpoint || strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	origin := u.Scheme + "://" + u.Host
	allowed := origin == strings.TrimRight(g.options.Builder.BaseURL, "/")
	for _, extra := range g.options.ResolverOrigins {
		if origin == strings.TrimRight(extra, "/") {
			allowed = true
		}
	}
	if !allowed {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || q.Get("apiKey") == "" {
		return false
	}
	for k, v := range q {
		if len(v) != 1 || (k != "apiKey" && k != "assetId" && k != "path" && k != "pickCode") {
			return false
		}
	}
	if id := q.Get("assetId"); id != "" {
		return g.options.OwnedAsset != nil && g.options.OwnedAsset(ctx, id)
	}
	return auth.Match(g.options.Builder.APIKey, q.Get("apiKey"))
}

// Only genuine format aliases are equivalent; MP4/remux/HLS is not TS.
func sameContainer(a, b string) bool {
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimPrefix(s, "."))
		if s == "mpegts" {
			return "ts"
		}
		return s
	}
	return normalize(a) == normalize(b)
}
func (g *Gateway) fallback(w http.ResponseWriter, r *http.Request, reason string) {
	if g.options.DirectPlayOnly {
		w.Header().Set("X-Media-Gateway-Reason", reason)
		if match := directOnlyMediaPath.FindStringSubmatch(gatewayRequestPath(r.URL.Path)); match != nil {
			g.rejectTranscoding(w, r, match[1])
		} else {
			gatewayError(w, 409, "direct_play_required", "仅直链播放：禁止服务器媒体中转")
		}
		return
	}
	w.Header().Set("X-Media-Gateway", "emby-upstream")
	w.Header().Set("X-Media-Gateway-Reason", reason)
	g.proxy.ServeHTTP(w, r)
}

// observeSession trusts only the successful upstream authentication response.
// Bodies are restored verbatim; credentials never enter logs or the store.
func (g *Gateway) observeSession(response *http.Response) error {
	if g.options.Upstream == nil {
		return nil
	}
	endpoint := strings.TrimPrefix(response.Request.URL.Path, strings.TrimRight(g.options.Upstream.Path, "/"))
	ctx, cancel := context.WithTimeout(response.Request.Context(), time.Second)
	defer cancel()
	token := callerToken(response.Request)
	if (strings.EqualFold(endpoint, "/Sessions/Logout") && response.StatusCode >= 200 && response.StatusCode < 300) || response.StatusCode == 401 {
		if token != "" {
			_ = g.options.Identities.Forget(ctx, IdentityKey(g.options.Upstream, token))
		}
	}
	if response.Request.Method != http.MethodPost || response.StatusCode != http.StatusOK || !strings.EqualFold(endpoint, "/Users/AuthenticateByName") {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
		return errors.New("Emby authentication response unavailable")
	}
	if len(raw) > 1<<20 {
		// Do not truncate a legitimate response, but refuse to learn oversized data.
		response.Body = &joinedReadCloser{Reader: io.MultiReader(bytes.NewReader(raw), response.Body), Closer: response.Body}
		return nil
	}
	deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	response.Body = io.NopCloser(bytes.NewReader(raw))
	var result struct {
		AccessToken string       `json:"AccessToken"`
		User        userIdentity `json:"User"`
	}
	if json.Unmarshal(raw, &result) == nil && result.AccessToken != "" && itemIDPattern.MatchString(result.User.ID) {
		_ = g.options.Identities.Remember(ctx, IdentityKey(g.options.Upstream, result.AccessToken), result.User.ID)
	}
	response.Header.Set("Cache-Control", "no-store")
	return nil
}

// Only public provider UA metadata is exposed to an authenticated native client.
// Cookies/authorization/additional provider headers are never copied into JSON.
func (g *Gateway) MetadataHeaders(ctx context.Context, raw string) map[string]string {
	if g.options.Playback == nil || !g.options.Enable302 {
		return nil
	}
	ref, err := g.reference(ctx, raw)
	if err != nil {
		return nil
	}
	return g.options.Playback.NativeClientHeaders(ctx, ref)
}

type directPlayOnlyKey struct{}

// All media output beneath a video/audio item is denied except explicitly
// authenticated original/Static routes. Subtitle/attachment APIs are not media
// encoding endpoints and retain their ordinary Emby permissions.
var directOnlyItemFilePath = regexp.MustCompile(`(?i)^/Items/([A-Za-z0-9_-]{1,128})/(?:Download|File)(?:[./]|$)`)
var directOnlyOpaqueMediaPath = regexp.MustCompile(`(?i)^/Sync(?:/[^/]+)*/(?:File|Download)(?:[./]|$)|^/LiveTv/(?:LiveStream(?:Files)?|Recording(?:s)?)(?:/|$)`)
var directOnlyMediaPath = regexp.MustCompile(`(?i)^/(?:Videos|Audio)/([A-Za-z0-9_-]{1,128})/(.*)$`)
var directOnlyMetadataPath = regexp.MustCompile(`(?i)^/Videos/[A-Za-z0-9_-]{1,128}/(?:Subtitles|Attachments|AdditionalParts)(?:/|$)`)

func (g *Gateway) rejectTranscoding(w http.ResponseWriter, r *http.Request, itemID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if _, _, err := g.authorizeItem(ctx, r, itemID); err != nil {
		var upstream *upstreamError
		if errors.As(err, &upstream) && (upstream.Status == 401 || upstream.Status == 403 || upstream.Status == 404) {
			gatewayError(w, upstream.Status, "emby_access_denied", "Emby拒绝当前用户读取此媒体")
		} else {
			gatewayError(w, 503, "emby_identity_unavailable", "无法确认播放权限；未转发媒体请求")
		}
		return
	}
	gatewayError(w, 409, "direct_play_required", "仅直链播放：禁止HLS、转码、封装转换和服务器媒体中转；客户端必须支持原始文件格式")
}

func (g *Gateway) DirectPlayOnly() bool { return g.options.DirectPlayOnly }

// Emby accepts /emby aliases too. Nested/case variants must not bypass a media
// policy merely because the public gateway adds its own mount prefix.
func gatewayRequestPath(raw string) string {
	raw = path.Clean(raw)
	for len(raw) >= 5 && strings.EqualFold(raw[:5], "/emby") && (len(raw) == 5 || raw[5] == '/') {
		if len(raw) == 5 {
			return "/"
		}
		raw = path.Clean(raw[5:])
	}
	if raw == "" {
		return "/"
	}
	return raw
}
