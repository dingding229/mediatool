package ffx

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type source struct {
	value, target string
	local         bool
	secrets       []string
}

func validTimeout(v time.Duration) bool { return v > 0 && v <= 2*time.Minute }
func plain(v string, max int) bool {
	return len(v) <= max && utf8.ValidString(v) && !strings.ContainsFunc(v, unicode.IsControl)
}
func validBinary(v string) bool { return v == "" || filepath.IsAbs(v) && plain(v, 4096) }
func inputSource(raw string, local bool) (source, error) {
	var s source
	if raw == "" || !plain(raw, 8192) {
		return s, ErrInvalid
	}
	if filepath.IsAbs(raw) && local {
		info, err := os.Stat(raw)
		if err != nil || !info.Mode().IsRegular() {
			return s, ErrInvalid
		}
		return source{value: filepath.Clean(raw), target: "local-test-file", local: true}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "\\") {
		return s, ErrInvalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return s, ErrInvalid
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return s, ErrInvalid
	}
	s.value = u.String()
	s.secrets = []string{u.RawQuery}
	for _, values := range query {
		for _, value := range values {
			if value != "" {
				s.secrets = append(s.secrets, value, url.QueryEscape(value))
			}
		}
	}
	u.RawQuery = ""
	u.ForceQuery = false
	s.target = u.String()
	return s, nil
}
func (r *Runner) arguments(link string, offset time.Duration, o Options, still bool) (source, []string, time.Duration, error) {
	s, err := inputSource(link, r.config.AllowLocalFiles)
	if err != nil {
		return s, nil, 0, err
	}
	timeout := r.config.ProbeTimeout
	if still {
		timeout = r.config.StillTimeout
	}
	if o.Timeout != 0 {
		timeout = o.Timeout
	}
	if !validTimeout(timeout) || !plain(o.UserAgent, 512) || offset < 0 || offset > 24*time.Hour {
		return s, nil, 0, ErrInvalid
	}
	if o.ProbeSize == 0 {
		o.ProbeSize = 8 << 20
	}
	if o.AnalyzeDuration == 0 {
		o.AnalyzeDuration = 5 * time.Second
	}
	if o.ProbeSize < 32768 || o.ProbeSize > 16<<20 || o.AnalyzeDuration < time.Millisecond || o.AnalyzeDuration > 30*time.Second {
		return s, nil, 0, ErrInvalid
	}
	protocols := "http,https,tcp,tls"
	if s.local {
		protocols += ",file"
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-protocol_whitelist", protocols, "-rw_timeout", strconv.FormatInt(min(timeout, 10*time.Second).Microseconds(), 10), "-probesize", strconv.FormatInt(o.ProbeSize, 10), "-analyzeduration", strconv.FormatInt(o.AnalyzeDuration.Microseconds(), 10)}
	if o.UserAgent != "" && !s.local {
		args = append(args, "-user_agent", o.UserAgent)
	}
	if !still {
		args = append(args, "-show_format", "-show_streams", "-show_chapters", "-print_format", "json", "-i", s.value)
		return s, args, timeout, nil
	}
	if o.Width == 0 {
		o.Width = 640
	}
	if o.Height == 0 {
		o.Height = 360
	}
	if o.Quality == 0 {
		o.Quality = 3
	}
	if o.Width < 2 || o.Width > 1920 || o.Height < 2 || o.Height > 1080 || o.Quality < 1 || o.Quality > 31 {
		return s, nil, 0, ErrInvalid
	}
	args = append(args, "-nostdin", "-ss", strconv.FormatFloat(offset.Seconds(), 'f', 6, 64), "-i", s.value, "-map", "0:v:0", "-map_metadata", "-1", "-frames:v", "1", "-an", "-sn", "-dn", "-vf", "scale="+strconv.Itoa(o.Width)+":"+strconv.Itoa(o.Height)+":force_original_aspect_ratio=decrease", "-threads", "1", "-c:v", "mjpeg", "-q:v", strconv.Itoa(o.Quality), "-f", "image2pipe", "pipe:1")
	return s, args, timeout, nil
}
