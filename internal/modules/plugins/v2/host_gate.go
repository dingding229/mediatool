package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"sync"
)

// A worker has one invocation lane. The host, not the SDK/request, installs its
// API execution scope ceiling. Live DB authorization is still checked per call.
type hostGate struct {
	mu         sync.RWMutex
	restricted bool
	scopes     map[string]bool
}

func (g *hostGate) enter(scopes []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.restricted = true
	g.scopes = map[string]bool{}
	for _, s := range scopes {
		g.scopes[s] = true
	}
}
func (g *hostGate) leave() { g.mu.Lock(); defer g.mu.Unlock(); g.restricted = false; g.scopes = nil }
func (s Store) scopedHost(l Lease, g *hostGate) func(context.Context, json.RawMessage) (any, *contracts.Error) {
	call := s.Host(l)
	return func(ctx context.Context, raw json.RawMessage) (any, *contracts.Error) {
		var c contracts.HostCall
		if contracts.Decode(raw, &c) != nil {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		g.mu.RLock()
		allowed := !g.restricted || g.scopes[hostScope(c.Method)]
		g.mu.RUnlock()
		if !allowed {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		return call(ctx, raw)
	}
}
