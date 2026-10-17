package dto

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// PlayRequest is THIS server's start-report body for /Sessions/Playing,
// corresponding to Emby's PlaybackStartInfo, not its remote-control PlayRequest.
// Start and progress share the common fields; no user identity is trusted here.
type PlayRequest PlaybackProgressInfo
type PlaybackProgressInfo struct {
	ItemId              string `json:"ItemId,omitempty"`
	MediaSourceId       string `json:"MediaSourceId,omitempty"`
	PlaySessionId       string `json:"PlaySessionId,omitempty"`
	SessionId           string `json:"SessionId,omitempty"`
	PositionTicks       *int64 `json:"PositionTicks,omitempty"`
	RunTimeTicks        *int64 `json:"RunTimeTicks,omitempty"`
	AudioStreamIndex    *int   `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex *int   `json:"SubtitleStreamIndex,omitempty"`
	VolumeLevel         *int   `json:"VolumeLevel,omitempty"`
	IsPaused            bool   `json:"IsPaused,omitempty"`
	IsMuted             bool   `json:"IsMuted,omitempty"`
	CanSeek             bool   `json:"CanSeek,omitempty"`
	PlayMethod          string `json:"PlayMethod,omitempty"`
}
type PlaybackStopInfo struct {
	ItemId        string `json:"ItemId,omitempty"`
	MediaSourceId string `json:"MediaSourceId,omitempty"`
	PlaySessionId string `json:"PlaySessionId,omitempty"`
	SessionId     string `json:"SessionId,omitempty"`
	PositionTicks *int64 `json:"PositionTicks,omitempty"`
	Failed        bool   `json:"Failed,omitempty"`
}

func (p *PlayRequest) UnmarshalJSON(raw []byte) error {
	type wire PlayRequest
	var value wire
	if err := decodeReport(raw, &value); err != nil {
		return err
	}
	*p = PlayRequest(value)
	return nil
}
func (p *PlaybackProgressInfo) UnmarshalJSON(raw []byte) error {
	type wire PlaybackProgressInfo
	var value wire
	if err := decodeReport(raw, &value); err != nil {
		return err
	}
	*p = PlaybackProgressInfo(value)
	return nil
}
func (p *PlaybackStopInfo) UnmarshalJSON(raw []byte) error {
	type wire PlaybackStopInfo
	var value wire
	if err := decodeReport(raw, &value); err != nil {
		return err
	}
	*p = PlaybackStopInfo(value)
	return nil
}

// Unknown fields are deliberately ignored. Only declared numeric fields are
// normalized; no float64 conversion, truncation, or string-to-bool coercion.
func decodeReport(raw []byte, target any) error {
	if len(raw) > 65536 {
		return ErrInvalid
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil || input == nil {
		return ErrInvalid
	}
	output := map[string]json.RawMessage{}
	typ := reflect.TypeOf(target).Elem()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		for key, value := range input {
			if !strings.EqualFold(key, name) {
				continue
			}
			kind := field.Type
			if kind.Kind() == reflect.Pointer {
				kind = kind.Elem()
			}
			if (kind.Kind() == reflect.Int || kind.Kind() == reflect.Int64) && !bytes.Equal(value, []byte("null")) {
				text := string(value)
				if len(value) > 0 && value[0] == '"' {
					if err := json.Unmarshal(value, &text); err != nil {
						return ErrInvalid
					}
				}
				bits := 64
				if kind.Kind() == reflect.Int {
					bits = 32
				}
				number, err := strconv.ParseInt(text, 10, bits)
				if err != nil {
					return ErrInvalid
				}
				value = []byte(strconv.FormatInt(number, 10))
			}
			if prior, exists := output[name]; exists && !bytes.Equal(prior, value) {
				return ErrInvalid
			}
			output[name] = value
		}
	}
	normalized, err := json.Marshal(output)
	if err != nil {
		return err
	}
	return json.Unmarshal(normalized, target)
}
