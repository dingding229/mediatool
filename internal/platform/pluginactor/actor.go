// Package pluginactor carries host-issued administrative plugin authority. It is
// not a media user, never decoded from RPC and never creates media credentials.
package pluginactor

import (
	"context"
	"errors"
	"slices"
)

var ErrDenied = errors.New("plugin actor denied")

type key struct{}
type Actor struct {
	instance, scope string
	libraries       []string
}

// With is called only after live manifest/grant/generation validation by Host.
func With(ctx context.Context, instance, scope string, libraries []string) context.Context {
	return context.WithValue(ctx, key{}, Actor{instance, scope, slices.Clone(libraries)})
}
func Require(ctx context.Context, scope string) (Actor, error) {
	a, ok := ctx.Value(key{}).(Actor)
	if !ok || a.instance == "" || a.scope != scope {
		return Actor{}, ErrDenied
	}
	return a, nil
}
func (a Actor) Libraries() []string { return slices.Clone(a.libraries) }
func (a Actor) Allows(id string) bool {
	return len(a.libraries) == 0 || slices.Contains(a.libraries, id)
}
func (a Actor) Restricted() bool { return len(a.libraries) > 0 }
