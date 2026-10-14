// Package mediausers owns member identities, never administrator credentials.
package mediausers

import (
	"errors"
	"media-tool/internal/modules/accounts"
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
	LibraryID string `json:"libraryId"`
	CanPlay   bool   `json:"canPlay"`
}
type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"displayName"`
	Enabled            bool      `json:"enabled"`
	MustChangePassword bool      `json:"mustChangePassword"`
	Revision           int64     `json:"revision"`
	Grants             []Grant   `json:"grants"`
	ActiveSessions     int       `json:"activeSessions"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
type Input struct {
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	Password    string  `json:"password"`
	Enabled     bool    `json:"enabled"`
	Revision    int64   `json:"revision"`
	Grants      []Grant `json:"grants"`
}

func (in *Input) Validate(create bool) error {
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
	if len(in.Grants) > 2 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, g := range in.Grants {
		if g.LibraryID != "movies" && g.LibraryID != "series" || seen[g.LibraryID] {
			return ErrInvalid
		}
		seen[g.LibraryID] = true
	}
	return nil
}
