package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"sync"
)

// A worker has one invocation lane. The host, not the SDK/request, installs its
// API execution scope ceiling. Live DB authorization is still checked per call.
type hostGate struct {
	mu         sync.RWMutex
	restricted bool
	action     bool
	scopes     map[string]bool
}

func (g *hostGate) enter(scopes []string) { g.enterFor(scopes, false) }
func (g *hostGate) enterFor(scopes []string, action bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.restricted = true
	g.action = action
	g.scopes = map[string]bool{}
	for _, s := range scopes {
		g.scopes[s] = true
	}
}
func (g *hostGate) leave() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.restricted = false
	g.action = false
	g.scopes = nil
}
func (s Store) scopedHost(l Lease, g *hostGate) func(context.Context, json.RawMessage) (any, *contracts.Error) {
	call := s.Host(l)
	return func(ctx context.Context, raw json.RawMessage) (any, *contracts.Error) {
		var c contracts.HostCall
		if contracts.Decode(raw, &c) != nil {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		g.mu.RLock()
		allowed := !g.restricted || g.scopes[hostScope(c.Method)]
		if g.restricted && g.action && c.Method == "workflows.run" {
			allowed = false
		} // No nested jobs may bypass the approved DAG depth/cycle bound.
		if g.restricted {
			for _, extra := range b.RequiredScopes(c.Method) {
				allowed = allowed && g.scopes[extra]
			}
		}
		g.mu.RUnlock()
		if !allowed {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		return call(ctx, raw)
	}
}
