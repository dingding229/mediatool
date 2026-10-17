// Package mediausers owns member identities, never administrator credentials.
package mediausers

import (
	"bytes"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/platform/auth"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrCredentials = errors.New("invalid media user credentials")
	ErrUnavailable = errors.New("media user database unavailable")
	ErrConflict    = errors.New("media user revision or username conflict")
	ErrNotFound    = errors.New("media user not found")
	ErrInvalid     = errors.New("invalid media user input")
)

type Grant struct {
	LibraryID   string `json:"libraryId"`
	CanPlay     bool   `json:"canPlay"`
	CanDownload bool   `json:"canDownload"`
}
type User struct {
	MaxStreams         int        `json:"maxStreams"`
	MaxParentalRating  *int       `json:"maxParentalRating"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Enabled            bool       `json:"enabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	Revision           int64      `json:"revision"`
	Grants             []Grant    `json:"grants"`
	ActiveSessions     int        `json:"activeSessions"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
type Input struct {
	MaxStreams             *int       `json:"maxStreams,omitempty"`
	MaxParentalRating      *int       `json:"maxParentalRating"`
	ExpiresAt              *time.Time `json:"expiresAt"`
	parentalSet, expirySet bool
	Username               string  `json:"username"`
	DisplayName            string  `json:"displayName"`
	Password               string  `json:"password"`
	Enabled                bool    `json:"enabled"`
	Revision               int64   `json:"revision"`
	Grants                 []Grant `json:"grants"`
	MustChangePassword     *bool   `json:"mustChangePassword,omitempty"`
}

// Preserve omitted nullable limits on PUT, while explicit null clears them.
func (in *Input) UnmarshalJSON(raw []byte) error {
	type alias Input
	var value alias
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	*in = Input(value)
	for key := range fields {
		if strings.EqualFold(key, "maxParentalRating") {
			in.parentalSet = true
		}
		if strings.EqualFold(key, "expiresAt") {
			in.expirySet = true
		}
	}
	return nil
}
func (in *Input) Validate(create bool) error {
	if in.MaxStreams != nil && (*in.MaxStreams < 1 || *in.MaxStreams > 20) || in.MaxParentalRating != nil && (*in.MaxParentalRating < 0 || *in.MaxParentalRating > 100) {
		return ErrInvalid
	}
	if in.ExpiresAt != nil {
		v := in.ExpiresAt.UTC()
		in.ExpiresAt = &v
	}

	var err error
	in.Username, err = accounts.NormalizeUsername(in.Username)
	if err != nil {
		return err
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !utf8.ValidString(in.DisplayName) || utf8.RuneCountInString(in.DisplayName) > 80 || strings.ContainsAny(in.DisplayName, "\x00\r\n") {
		return ErrInvalid
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Username
	}
	if create {
		if err = auth.ValidatePassword(in.Password); err != nil {
			return err
		}
	} else if in.Revision < 1 || in.Password != "" {
		return ErrInvalid
	}
	if len(in.Grants) > 200 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, g := range in.Grants {
		if !(nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: g.LibraryID}).Valid() || seen[g.LibraryID] || g.CanDownload && !g.CanPlay {
			return ErrInvalid
		}
		seen[g.LibraryID] = true
	}
	return nil
}
