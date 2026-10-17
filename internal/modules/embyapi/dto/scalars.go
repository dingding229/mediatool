// Package dto defines the native server's Emby JSON contract only. It does not
// query storage, authenticate users, bind listeners, or process media.
package dto

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid Emby DTO")

// Array serializes even a nil value as [], never null. Optional empty arrays may
// still be omitted with omitempty. No constructor is needed to make arrays safe.
type Array[T any] []T

func (a Array[T]) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(a))
}

// UTCDateTime uses one wire format: UTC, exactly seven fractional digits, Z.
// NewUTCDateTime explicitly normalizes to Emby's 100 ns tick precision.
type UTCDateTime struct{ value time.Time }

func NewUTCDateTime(t time.Time) UTCDateTime {
	return UTCDateTime{t.UTC().Truncate(100 * time.Nanosecond)}
}
func (d UTCDateTime) Time() time.Time { return d.value }
func (d UTCDateTime) MarshalJSON() ([]byte, error) {
	if d.value.Year() < 1 || d.value.Year() > 9999 {
		return nil, ErrInvalid
	}
	return json.Marshal(d.value.UTC().Format("2006-01-02T15:04:05.0000000Z"))
}
func (d *UTCDateTime) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ErrInvalid
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	t = t.UTC()
	if err != nil || t.Year() < 1 || t.Year() > 9999 || t.Nanosecond()%100 != 0 {
		return ErrInvalid
	}
	*d = NewUTCDateTime(t)
	return nil
}
