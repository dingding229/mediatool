package nativeplayback

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"media-tool/internal/platform/mediaext"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const SessionTTL = 5 * time.Minute

// A playback identity comes only from T41, never from the request UserId.
type Actor struct {
	User                      contracts.User
	Device                    contracts.Device
	Permissions               contracts.PermissionChecker
	Token                     string `json:"-"`
	Origin, UserAgent, Method string
	Address                   clientinfo.Address
}
type Request struct {
	ItemID, MediaSourceID      string
	SubtitleIndex              *int
	SubtitleID, SubtitleFormat string
	Download                   bool
}
type Service interface {
	Info(context.Context, Actor, Request) (dto.PlaybackInfoResponse, error)
	Stream(context.Context, Actor, Request) (RedirectTarget, error)
}
type RetryError struct{ After time.Duration }

func (*RetryError) Error() string { return "native playback rate limited" }
func (*RetryError) Unwrap() error { return ErrUpstream }

var ErrSubtitleConversion = errors.New("external subtitle conversion unsupported")
var ErrInvalid = errors.New("invalid native playback request")
var digits = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var sourceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,512}$`)

// StreamPath is the credential-free native media URL constructor. Unknown containers use
// mkv: a neutral original-container hint, never a request to remux/transcode.
func StreamPath(origin, item, source, container string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || !digits.MatchString(item) || !sourceID.MatchString(source) {
		return "", ErrInvalid
	}
	ext := strings.ToLower(container)
	switch ext {
	case "mkv", "mp4", "m4v", "mov", "avi", "ts", "m2ts", "webm", "mpeg", "mpg", "wmv", "flv":
	default:
		ext = "mkv"
	}
	q := url.Values{"Static": {"true"}, "MediaSourceId": {source}}
	relative := "/Videos/" + item + "/stream." + ext + "?" + q.Encode()
	absolute := u.Scheme + "://" + u.Host + relative
	if _, err = dto.NewMediaSourceInfo(absolute); err != nil {
		return "", ErrInvalid
	}
	return absolute, nil
}

// StreamURLs separates the credential-free display Path from the existing
// user-scoped DirectStreamUrl. Neither URL contains a per-file bearer grant.
func StreamURLs(origin, item, source, container, token string) (string, string, error) {
	if !sourceID.MatchString(token) {
		return "", "", ErrInvalid
	}
	absolute, err := StreamPath(origin, item, source, container)
	if err != nil {
		return "", "", err
	}
	direct, _ := url.Parse(absolute)
	q := direct.Query()
	q.Set("api_key", token)
	direct.RawQuery = q.Encode()
	return absolute, direct.RequestURI(), nil
}

// Subtitle URLs expose only an authenticated native endpoint, never the cloud path.
func SubtitleURL(origin, item, source, token, id string, index int, ext string) (string, error) {
	absolute, _, e := StreamURLs(origin, item, source, "mkv", token)
	if e != nil || !sourceID.MatchString(id) || index < 0 || !mediaext.IsSubtitle("x."+ext) {
		return "", ErrInvalid
	}
	u, _ := url.Parse(absolute)
	u.Path = "/Videos/" + item + "/" + source + "/Subtitles/" + strconv.Itoa(index) + "/Stream." + ext
	q := u.Query()
	q.Set("SubtitleId", id)
	q.Set("api_key", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
