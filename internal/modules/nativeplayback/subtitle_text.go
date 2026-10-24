package nativeplayback

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// TextSubtitle is the sole direct-content exception: a registered upload,
// already authenticated and checked by the subtitle store. Not a media reader.
// Private fields force every producer through the bounded constructor.
type TextSubtitle struct {
	body []byte
	mime string
}

func NewTextSubtitle(ext string, body []byte) (*TextSubtitle, error) {
	mime := ""
	switch ext {
	case "srt":
		mime = "application/x-subrip; charset=utf-8"
	case "ass", "ssa":
		mime = "text/x-ssa; charset=utf-8"
	case "vtt":
		mime = "text/vtt; charset=utf-8"
	}
	valid := bytes.Contains(body, []byte("-->"))
	if ext == "ass" || ext == "ssa" {
		valid = strings.Contains(strings.ToLower(string(body)), "[events]")
	}
	if !valid || bytes.IndexByte(body, 0) >= 0 || mime == "" || len(body) == 0 || len(body) > 5<<20 || !utf8.Valid(body) {
		return nil, ErrInvalid
	}
	return &TextSubtitle{body: body, mime: mime}, nil
}
func (s *TextSubtitle) Bytes() []byte       { return s.body }
func (s *TextSubtitle) ContentType() string { return s.mime }
