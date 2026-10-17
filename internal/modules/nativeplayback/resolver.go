// Package nativeplayback defines a redirect-only boundary. It cannot return media
// bytes, readers, HLS playlists, or a transcoder. T32 implements this contract.
package nativeplayback

import (
	"context"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"time"
)

var (
	ErrNotImplemented = errors.New("native playback not implemented")
	ErrForbidden      = errors.New("native playback permission denied")
	ErrParentalDenied = errors.New("native playback parental rating denied")
	ErrUADenied       = errors.New("native playback UA denied")
	ErrStreamLimit    = errors.New("native playback stream limit exceeded")
	ErrNotFound       = errors.New("native playback item not found")
	ErrUpstream       = errors.New("native playback upstream unavailable")
)

// This internal target is consumed only as a 302 Location, never a JSON DTO.
type RedirectTarget struct {
	URL       string    `json:"-"`
	ExpiresAt time.Time `json:"-"`
}
type StreamResolver interface {
	Resolve(ctx context.Context, user contracts.User, itemID, mediaSourceID, ua, clientIP string) (RedirectTarget, error)
}
type empty struct{}

func NewStreamResolver() StreamResolver { return empty{} }
func (empty) Resolve(context.Context, contracts.User, string, string, string, string) (RedirectTarget, error) {
	return RedirectTarget{}, ErrNotImplemented
}

// PermissionError is the explicit browse→play boundary; generic denial remains
// opaque (404) until a playback handler chooses this mapping (403 + reason).
func PermissionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, contracts.ErrParentalDenied):
		return ErrParentalDenied
	case errors.Is(err, contracts.ErrStreamLimit):
		return ErrStreamLimit
	case errors.Is(err, contracts.ErrForbidden), errors.Is(err, contracts.ErrUnauthenticated):
		return ErrForbidden
	case errors.Is(err, contracts.ErrUnavailable):
		return ErrUpstream
	default:
		return err
	}
}
