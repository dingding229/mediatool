// Package embyapi handles protocol requests through domain contracts. It never
// accesses a database, registers a listener, or resolves/relays media itself.
package embyapi

import (
	"context"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"net/http"
)

type UserIdentity = contracts.User
type DeviceIdentity = contracts.Device
type TokenIdentity = contracts.Token

// Verified identities remain nil until T41 validates the token and versions.
// This internal envelope is never a response DTO (including Auth.UserID).
type RequestContext struct {
	Permissions contracts.PermissionChecker `json:"-"`
	Auth        Auth                        `json:"-"`
	Method      string                      `json:"-"`
	UserAgent   string                      `json:"-"`
	Address     clientinfo.Address          `json:"-"`
	Parameters  Values                      `json:"-"`
	User        *UserIdentity               `json:"-"`
	Device      *DeviceIdentity             `json:"-"`
	Token       *TokenIdentity              `json:"-"`
}
type contextKey struct{}

func RequestInfo(ctx context.Context) (RequestContext, bool) {
	info, ok := ctx.Value(contextKey{}).(RequestContext)
	return info, ok
}
func WithRequestInfo(r *http.Request, info RequestContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), contextKey{}, info))
}
func NewRequestContext(r *http.Request, proxies []string) (RequestContext, error) {
	var info RequestContext
	if len(r.Header.Values("User-Agent")) > 1 || !clientinfo.ValidUA(r.UserAgent()) || !clientinfo.ValidateProxies(proxies) {
		return info, ErrInvalid
	}
	values, err := ParseValues(r.URL.RawQuery)
	if err != nil {
		return info, err
	}
	auth, err := ParseAuth(r, values)
	if err != nil {
		return info, err
	}
	info.Auth, info.UserAgent, info.Parameters = auth, r.UserAgent(), values
	info.Method = r.Method
	info.Address = clientinfo.ClientAddress(r, proxies)
	return info, nil
}
