package embyapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"net/http"
	"strconv"
	"strings"
)

func playbackParameters(r *http.Request, in Values) (Values, error) {
	v := Values{entries: map[string][]string{}}
	for k, a := range in.entries {
		v.entries[k] = append([]string(nil), a...)
	}
	if r.Method != "POST" || r.ContentLength == 0 {
		return v, nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 256<<10))
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return v, ErrInvalid
	}
	seen := map[string]string{}
	for dec.More() {
		name, err := dec.Token()
		if err != nil {
			return v, ErrInvalid
		}
		key, ok := name.(string)
		if !ok || len(key) > 128 || len(seen) >= 128 {
			return v, ErrInvalid
		}
		var raw json.RawMessage
		if dec.Decode(&raw) != nil {
			return v, ErrInvalid
		}
		key = foldKey(key)
		var compact bytes.Buffer
		if json.Compact(&compact, raw) != nil {
			return v, ErrInvalid
		}
		text := compact.String()
		if old, ok := seen[key]; ok && old != text {
			return v, ErrInvalid
		}
		seen[key] = text
		if key == "deviceprofile" {
			if text != "null" && !strings.HasPrefix(text, "{") {
				return v, ErrInvalid
			}
			continue
		}
		switch key {
		case "playbackpositionticks", "playcount", "played", "isfavorite", "lastplayeddate", "itemid", "positionticks", "ispaused", "playsessionid", "eventname", "mediasourceid", "userid", "starttimeticks", "maxstreamingbitrate", "audiostreamindex", "subtitlestreamindex", "enabledirectplay", "enabledirectstream", "enabletranscoding":
		default:
			continue
		}
		// Emby nullable request properties commonly serialize as explicit null.
		// They mean unspecified, never a different user or an index of zero.
		if text == "null" {
			continue
		}
		if len(raw) > 2048 {
			return v, ErrInvalid
		}
		if strings.HasPrefix(text, `"`) {
			if json.Unmarshal(raw, &text) != nil {
				return v, ErrInvalid
			}
		}
		v.entries[key] = append(v.entries[key], text)
	}
	if _, err = dec.Token(); err != nil {
		return v, ErrInvalid
	}
	var extra any
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		return v, ErrInvalid
	}
	return v, nil
}
func playbackQuery(v Values) (string, string, error) {
	source, err := v.Scalar("MediaSourceId")
	if err != nil || len(source) > 512 {
		return "", "", ErrInvalid
	}
	user, err := v.Scalar("UserId")
	if err != nil || len(user) > 64 {
		return "", "", ErrInvalid
	}
	for _, key := range []string{"StartTimeTicks", "MaxStreamingBitrate", "AudioStreamIndex", "SubtitleStreamIndex"} {
		raw, e := v.Scalar(key)
		if e != nil {
			return "", "", ErrInvalid
		}
		if raw == "" {
			continue
		}
		n, e := strconv.ParseInt(raw, 10, 64)
		minimum := int64(0)
		maximum := int64(math.MaxInt64)
		if key == "AudioStreamIndex" || key == "SubtitleStreamIndex" {
			minimum = -1
			maximum = 65535
			if key == "SubtitleStreamIndex" {
				maximum = 2147483647
			}
		}
		if e != nil || n < minimum || n > maximum {
			return "", "", ErrInvalid
		}
	}
	for _, key := range []string{"EnableDirectPlay", "EnableDirectStream", "EnableTranscoding"} {
		if _, e := v.Boolean(key); e != nil {
			return "", "", ErrInvalid
		}
	}
	// Query DeviceProfile is JSON too. Profiles are accepted only as bounded data.
	if raw, e := v.Scalar("DeviceProfile"); e != nil || raw != "" && (!json.Valid([]byte(raw)) || !strings.HasPrefix(strings.TrimSpace(raw), "{")) {
		return "", "", ErrInvalid
	}
	return source, user, nil
}
func (s *Server) playback(w http.ResponseWriter, r *http.Request, info RequestContext, stream bool) {
	if s.options.Playback == nil || info.User == nil || info.Device == nil || info.Permissions == nil {
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
	request := nativeplayback.Request{ItemID: r.PathValue("id"), MediaSourceID: source, Download: strings.HasSuffix(strings.ToLower(r.URL.Path), "/download")}
	if raw := r.PathValue("index"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 || n > 2147483647 {
			s.router.fail(w, 400)
			return
		}
		request.SubtitleIndex = &n
		request.SubtitleFormat = strings.ToLower(r.PathValue("format"))
		if sourcePath := r.PathValue("mediaSourceId"); sourcePath != "" {
			if source != "" && source != sourcePath {
				s.router.fail(w, 400)
				return
			}
			request.MediaSourceID = sourcePath
		}
		request.SubtitleID, err = values.Scalar("SubtitleId")
		if err != nil || request.SubtitleID != "" && !opaqueID(request.SubtitleID) {
			s.router.fail(w, 400)
			return
		}
	}

	actor := nativeplayback.Actor{User: *info.User, Device: *info.Device, Permissions: info.Permissions, Token: info.Auth.Token, UserAgent: info.UserAgent, Address: info.Address, Method: info.Method}
	if stream {
		static, e := values.Boolean("Static")
		if e != nil {
			s.router.fail(w, 400)
			return
		}
		unsupported := static != nil && !*static
		for _, key := range []string{"VideoCodec", "AudioCodec", "TranscodingProtocol", "TranscodingContainer", "SegmentContainer"} {
			raw, e := values.Scalar(key)
			if e != nil {
				s.router.fail(w, 400)
				return
			}
			unsupported = unsupported || raw != ""
		}
		ext := strings.ToLower(r.PathValue("ext"))
		if ext == "m3u8" || ext == "mpd" || unsupported {
			writeAuthError(w, 400, "仅支持原始媒体及字幕直链，不支持格式转换、转码、HLS或DASH")
			return
		}
		target, err := s.options.Playback.Stream(r.Context(), actor, request)
		if err != nil {
			s.playbackError(w, err)
			return
		}
		if target.Subtitle != nil {
			// Only an authenticated subtitle route can consume bounded uploaded text.
			if request.SubtitleIndex == nil || target.URL != "" || len(target.Subtitle.Bytes()) > 5<<20 {
				s.router.fail(w, 503)
				return
			}
			w.Header().Set("Content-Type", target.Subtitle.ContentType())
			w.Header().Set("Content-Length", strconv.Itoa(len(target.Subtitle.Bytes())))
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			w.WriteHeader(http.StatusOK)
			if r.Method != "HEAD" {
				if _, err := w.Write(target.Subtitle.Bytes()); err != nil {
					s.options.Logger.Error("uploaded subtitle response failed", "module", "playback")
				}
			}
			return
		}
		// Never use http.Redirect: its default GET response contains an HTML body.
		w.Header().Set("Location", target.URL)
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusFound)
		requestURL := *r.URL
		safeQuery := requestURL.Query()
		for key := range safeQuery {
			switch strings.ToLower(key) {
			case "api_key", "apikey", "token", "x-emby-token", "x-mediabrowser-token", "authorization":
				safeQuery.Set(key, "[redacted]")
			}
		}
		requestURL.RawQuery = safeQuery.Encode()
		requestURL.Host = r.Host
		requestURL.Scheme = "http"
		if r.TLS != nil {
			requestURL.Scheme = "https"
		}
		cleanLog := func(value string) string { return strings.ReplaceAll(value, info.Auth.Token, "[redacted]") }
		s.options.Logger.Info("HTTP request", "module", "playback", "url", cleanLog(requestURL.String()), "path", cleanLog(r.URL.Path), "location", cleanLog(target.URL), "method", actor.Method, "status", 302, "responseBytes", 0, "userAgent", cleanLog(info.UserAgent), "clientIp", info.Address.ClientIP)
		return
	}
	settings, _ := r.Context().Value(serverSettingsKey{}).(nativelibrary.ServerSettings)
	actor.Origin, err = s.configuredMediaOrigin(settings)
	if err != nil {
		s.router.fail(w, 503)
		return
	}
	out, err := s.options.Playback.Info(r.Context(), actor, request)
	if err != nil {
		s.playbackError(w, err)
		return
	}
	s.write(w, out)
}
func (s *Server) playbackError(w http.ResponseWriter, err error) {
	var retry *nativeplayback.RetryError
	if errors.As(err, &retry) {
		seconds := max(1, int(math.Ceil(retry.After.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		s.router.fail(w, 429)
		return
	}
	if errors.Is(err, nativeplayback.ErrSubtitleConversion) {
		writeAuthError(w, 400, "仅支持原始字幕直链，此客户端请求的字幕格式转换不受支持")
		return
	}
	if errors.Is(err, nativeplayback.ErrInvalid) {
		s.router.fail(w, 400)
		return
	}
	message := ""
	switch {
	case errors.Is(err, nativeplayback.ErrForbidden):
		message = "权限不足，无法播放或下载此媒体库"
	case errors.Is(err, nativeplayback.ErrUADenied):
		message = "当前播放器UA未通过准入策略，请联系管理员"
	case errors.Is(err, nativeplayback.ErrParentalDenied):
		message = "分级限制，不允许播放此内容"
	case errors.Is(err, nativeplayback.ErrStreamLimit):
		message = "超过同时播放数限制，请先停止其他播放"
	}
	if message != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(403)
		if e := json.NewEncoder(w).Encode(struct {
			ErrorCode, Message string
			ResponseStatus     struct{ ErrorCode, Message string }
		}{"NotAllowed", message, struct{ ErrorCode, Message string }{"NotAllowed", message}}); e != nil {
			s.options.Logger.Debug("Native playback response interrupted")
		}
		return
	}
	status := DomainErrorStatus(err)
	if status == 500 {
		status = 503
	}
	s.router.fail(w, status)
}

// Public discovery retains its existing origin fallback. MediaSource URLs
// instead require configuredMediaOrigin and never inherit request headers.
func (s *Server) publicOrigin(r *http.Request, value nativelibrary.ServerSettings) (string, error) {
	origin := value.PublicURL
	if origin == "" {
		origin = s.options.PublicURL
	}
	if origin == "" {
		s.fallbackWarning.Do(func() {
			s.options.Logger.Warn("Native media public address is not configured; using trusted request origin", "module", "native_media")
		})
		if s.options.RequestOrigin != nil {
			var err error
			origin, err = s.options.RequestOrigin(r)
			if err != nil {
				return "", err
			}
		} else {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			origin = scheme + "://" + r.Host
		}
	}
	if !validOrigin(origin) {
		return "", ErrInvalid
	}
	return origin, nil
}

// Media URLs must never inherit request Host / X-Forwarded-* values.
func (s *Server) configuredMediaOrigin(settings nativelibrary.ServerSettings) (string, error) {
	origin := settings.PublicURL
	if origin == "" {
		origin = s.options.PublicURL
	}
	if !validOrigin(origin) {
		return "", ErrInvalid
	}
	return origin, nil
}
