// Package contracts isolates native authentication from the existing SQL repository.
// All identities are internal, verified values, never response DTOs or raw tokens.
package contracts

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotImplemented         = errors.New("native authentication not implemented")
	ErrUnauthenticated        = errors.New("native token rejected")
	ErrForbidden              = errors.New("native permission denied")
	ErrStreamLimit            = errors.New("native stream limit exceeded")
	ErrUnavailable            = errors.New("native authentication unavailable")
	ErrPasswordChangeRequired = errors.New("administrator must set a final password")
	ErrInvalidPassword        = errors.New("invalid new password")
	ErrRateLimited            = errors.New("native login rate limited")
)

type User struct {
	ID                   string
	Name                 string
	MustChangePassword   bool
	CanPlay, CanDownload bool
	CredentialVersion    int64
	Enabled              bool
	MaxStreams           int
	ExpiresAt            *time.Time
}
type Device struct{ ID, UserID, DeviceKey string }
type Token struct {
	Digest            [32]byte
	CredentialVersion int64
	ExpiresAt         time.Time
}
type DeviceInfo struct{ DeviceID, Name, Client, Version, UserAgent, ClientIP string }
type ItemAccess struct {
	ID, LibraryID  string
	LibraryIDs     []string
	OfficialRating string
}
type StreamCapacity struct{ Limit, InUse int }
type TokenAuthenticator interface {
	Authenticate(context.Context, string, DeviceInfo) (User, Device, Token, error)
}

// Login is the only boundary returning a newly issued plaintext token. This
// envelope is never marshaled/logged; the protocol explicitly builds its reply.
type LoginResult struct {
	User        User
	Device      Device
	Token       Token
	AccessToken string `json:"-"`
}
type PublicUser struct {
	Name, Id    string
	HasPassword bool
}
type AccountService interface {
	TokenAuthenticator
	Login(context.Context, string, string, DeviceInfo) (LoginResult, error)
	ChangePassword(context.Context, User, Token, string, string) error
	Logout(context.Context, Token) error
	PublicUsers(context.Context) ([]PublicUser, error)
	GetUser(context.Context, string) (User, error)
}

// All checks fail closed. StreamSlots is a snapshot, NOT an atomic reservation;
// T34 must enforce the actual limit while opening a playback session.
type PermissionChecker interface {
	VisibleLibraries(context.Context, User) ([]string, error)
	ItemAllowed(context.Context, User, ItemAccess) error
	CanBrowse(context.Context, User, string) error
	CanPlay(context.Context, User, string) error
	CanDownload(context.Context, User, string) error
	StreamSlots(context.Context, User) (StreamCapacity, error)
}
type empty struct{}

func NewTokenAuthenticator() TokenAuthenticator { return empty{} }
func NewPermissionChecker() PermissionChecker   { return empty{} }
func (empty) Authenticate(context.Context, string, DeviceInfo) (User, Device, Token, error) {
	return User{}, Device{}, Token{}, ErrNotImplemented
}
func (empty) CanBrowse(context.Context, User, string) error   { return ErrNotImplemented }
func (empty) CanPlay(context.Context, User, string) error     { return ErrNotImplemented }
func (empty) CanDownload(context.Context, User, string) error { return ErrNotImplemented }
func (empty) StreamSlots(context.Context, User) (StreamCapacity, error) {
	return StreamCapacity{}, ErrNotImplemented
}

func (empty) VisibleLibraries(context.Context, User) ([]string, error) { return nil, ErrNotImplemented }
func (empty) ItemAllowed(context.Context, User, ItemAccess) error      { return ErrNotImplemented }
