package nativeplayback

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"net/url"
	"regexp"
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
	ItemID, MediaSourceID string
	Download              bool
}
type Service interface {
	Info(context.Context, Actor, Request) (dto.PlaybackInfoResponse, error)
	Stream(context.Context, Actor, Request) (RedirectTarget, error)
}
type RetryError struct{ After time.Duration }

func (*RetryError) Error() string { return "native playback rate limited" }
func (*RetryError) Unwrap() error { return ErrUpstream }

var ErrInvalid = errors.New("invalid native playback request")
var digits = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var sourceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,512}$`)

// StreamURLs is the only media delivery URL constructor. Unknown containers use
// mkv: a neutral original-container hint, never a request to remux/transcode.
func StreamURLs(origin, item, source, container, token string) (string, string, error) {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || !digits.MatchString(item) || !sourceID.MatchString(source) || !sourceID.MatchString(token) {
		return "", "", ErrInvalid
	}
	ext := strings.ToLower(container)
	switch ext {
	case "mkv", "mp4", "m4v", "mov", "avi", "ts", "m2ts", "webm", "mpeg", "mpg", "wmv", "flv":
	default:
		ext = "mkv"
	}
	q := url.Values{"Static": {"true"}, "MediaSourceId": {source}, "api_key": {token}}
	relative := "/Videos/" + item + "/stream." + ext + "?" + q.Encode()
	absolute := u.Scheme + "://" + u.Host + relative
	if _, err = dto.NewMediaSourceInfo(absolute); err != nil {
		return "", "", ErrInvalid
	}
	return absolute, relative, nil
}
