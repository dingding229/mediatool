// Package probeinfo is a pure, bounded data boundary. It performs no I/O and
// never retains filenames, arbitrary tags, side data or raw upstream objects.
package probeinfo

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion  = 1
	MaxStreams     = 64
	MaxInputBytes  = 8 << 20
	MaxStreamBytes = 64 << 10
	Video          = "Video"
	Audio          = "Audio"
	Subtitle       = "Subtitle"
	// Wire contract fixed in T24-B. T52's local reports are Emby4.10 summary
	// facts, not 4.8 HDR samples; keep the approved pairs until actual evidence
	// exists (see NATIVE_CLIENT_SEQUENCE_MATRIX), never infer a server enum from
	// a display title or rename DOVI to DolbyVision based on absent samples.
	SDR           = "SDR"
	HDR10         = "HDR10"
	HLG           = "HLG"
	HDR           = "HDR"
	DOVI          = "DOVI"
	DOVIWithHDR10 = "DOVIWithHDR10"
	DOVIWithHLG   = "DOVIWithHLG"
	DOVIWithSDR   = "DOVIWithSDR"
)

var ErrInvalid = errors.New("invalid media probe result")

type Result struct {
	SchemaVersion int      `json:"schema_version,omitempty"`
	DurationTicks int64    `json:"durationTicks"`
	Container     string   `json:"container,omitempty"`
	BitRate       *int64   `json:"bitRate,omitempty"`
	Size          *int64   `json:"size,omitempty"`
	Streams       []Stream `json:"streams,omitempty"`
}
type Stream struct {
	Index                int      `json:"index"`
	Type                 string   `json:"type"`
	Codec                string   `json:"codec,omitempty"`
	Profile              string   `json:"profile,omitempty"`
	Level                *int     `json:"level,omitempty"`
	Width                *int     `json:"width,omitempty"`
	Height               *int     `json:"height,omitempty"`
	BitRate              *int64   `json:"bitRate,omitempty"`
	AverageFrameRate     *float32 `json:"averageFrameRate,omitempty"`
	RealFrameRate        *float32 `json:"realFrameRate,omitempty"`
	PixelFormat          string   `json:"pixelFormat,omitempty"`
	BitDepth             *int     `json:"bitDepth,omitempty"`
	AspectRatio          string   `json:"aspectRatio,omitempty"`
	VideoRange           string   `json:"videoRange,omitempty"`
	VideoRangeType       string   `json:"videoRangeType,omitempty"`
	IsInterlaced         bool     `json:"isInterlaced,omitempty"`
	Rotation             *int     `json:"rotation,omitempty"`
	Channels             *int     `json:"channels,omitempty"`
	ChannelLayout        string   `json:"channelLayout,omitempty"`
	SampleRate           *int     `json:"sampleRate,omitempty"`
	Language             string   `json:"language,omitempty"`
	Title                string   `json:"title,omitempty"`
	IsDefault            bool     `json:"isDefault,omitempty"`
	IsForced             bool     `json:"isForced,omitempty"`
	IsHearingImpaired    bool     `json:"isHearingImpaired,omitempty"`
	IsTextSubtitleStream bool     `json:"isTextSubtitleStream,omitempty"`
	DisplayTitle         string   `json:"displayTitle,omitempty"`
}

func clean(s string, limit int) string {
	if len(s) > limit || !utf8.ValidString(s) || strings.ContainsAny(s, "/\\?&=%") || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
		return ""
	}
	lower := strings.ToLower(s)
	for _, marker := range []string{"http:", "https:", "file:", "smb:", "pickcode", "pick_code", "strm302", "api_key", "apikey", "115cdn"} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return s
}

// Validate also protects persistence against an incorrectly implemented Parser.
// Version zero is readable for legacy duration-only snapshots; new writes require 1.
func (r Result) Validate() error {
	if r.SchemaVersion < 0 || r.SchemaVersion > SchemaVersion || r.DurationTicks < 0 || len(r.Streams) > MaxStreams {
		return ErrInvalid
	}
	seen := map[int]bool{}
	for _, s := range r.Streams {
		if s.Index < 0 || s.Index > 65535 || seen[s.Index] || (s.Type != Video && s.Type != Audio && s.Type != Subtitle) {
			return ErrInvalid
		}
		seen[s.Index] = true
		switch s.VideoRange {
		case "":
			if s.VideoRangeType != "" {
				return ErrInvalid
			}
		case SDR:
			if s.VideoRangeType != SDR {
				return ErrInvalid
			}
		case HDR:
			switch s.VideoRangeType {
			case HDR10, HLG, DOVI, DOVIWithHDR10, DOVIWithHLG, DOVIWithSDR:
			default:
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	// Walk only this finite typed schema, never untrusted maps/objects.
	var valid func(reflect.Value, string) bool
	valid = func(v reflect.Value, name string) bool {
		if v.Kind() == reflect.Pointer {
			return v.IsNil() || valid(v.Elem(), name)
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if !valid(v.Field(i), v.Type().Field(i).Name) {
					return false
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if !valid(v.Index(i), name) {
					return false
				}
			}
		case reflect.String:
			limit := 64
			if name == "Codec" {
				limit = 32
			}
			if name == "Title" || name == "DisplayTitle" {
				limit = 256
			}
			return clean(v.String(), limit) == v.String()
		case reflect.Int, reflect.Int64:
			n := v.Int()
			switch name {
			case "Width", "Height":
				return n >= 1 && n <= 32768
			case "Channels", "BitDepth":
				return n >= 1 && n <= 64
			case "SampleRate":
				return n >= 1 && n <= 768000
			case "Level":
				return n >= 0 && n <= 1000
			}
			if name == "Rotation" {
				return n >= -360 && n <= 360
			}
			return n >= 0
		case reflect.Float32:
			n := v.Float()
			return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 1000
		}
		return true
	}
	if !valid(reflect.ValueOf(r), "") {
		return ErrInvalid
	}
	return nil
}
func (r Result) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	type wire Result
	return json.Marshal(wire(r))
}
