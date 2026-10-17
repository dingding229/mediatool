// Package v2 implements built-in plugin instances. It never imports the WASM runner.
package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/registry"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"runtime/debug"
	"slices"
)

type Desired string

const (
	Enabled  Desired = "enabled"
	Disabled Desired = "disabled"
	Absent   Desired = "absent"
)

type State string

const (
	Stopped     State = "stopped"
	Starting    State = "starting"
	Running     State = "running"
	Stopping    State = "stopping"
	Backoff     State = "backoff"
	NeedsAction State = "needs_action"
)

type Availability string

const (
	Available   Availability = "available"
	Missing     Availability = "missing"
	Unsupported Availability = "unsupported"
)

type Reason string

const (
	NoReason              Reason = ""
	WorkerFailed          Reason = "worker_failed"
	ResourceLimit         Reason = "resource_limit"
	PermissionReview      Reason = "permission_review_required"
	DefinitionMissing     Reason = "definition_missing"
	DefinitionUnsupported Reason = "definition_unsupported"
	DataUnavailable       Reason = "data_unavailable"
)

type Operation string

const (
	Install   Operation = "install"
	Enable    Operation = "enable"
	Disable   Operation = "disable"
	Clone     Operation = "clone"
	Configure Operation = "configure"
	Review    Operation = "review"
	Remove    Operation = "remove"
)

var (
	ErrInvalid     = errors.New("invalid plugin request")
	ErrConflict    = errors.New("plugin revision conflict")
	ErrUnavailable = errors.New("plugin unavailable")
	ErrNotFound    = errors.New("plugin not found")
)

type Definition struct {
	Manifest    contracts.Manifest `json:"manifest"`
	Fingerprint string             `json:"fingerprint"`
}
type Instance struct {
	ID       string  `json:"id"`
	PluginID string  `json:"pluginId"`
	Desired  Desired `json:"desiredState"`
	State    State   `json:"state"`
	Revision int64   `json:"revision"`
	Reason   Reason  `json:"reason"`
}
type Request struct {
	OperationKey     string          `json:"operationKey"`
	Kind             Operation       `json:"kind"`
	InstanceID       string          `json:"instanceId,omitempty"`
	PluginID         string          `json:"pluginId,omitempty"`
	Fingerprint      string          `json:"fingerprint"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Confirm          bool            `json:"confirm"`
	Scopes           []string        `json:"scopes"`
	Config           json.RawMessage `json:"config,omitempty"`
}

func BuildID() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/dingding229/mediatool-plugin" {
				return d.Version
			}
		}
	}
	return "v0.0.0-20261017132453-f259a018ba77"
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Catalog() ([]Definition, error) {
	defs, e := registry.All()
	if e != nil {
		return nil, ErrUnavailable
	}
	out := make([]Definition, 0, len(defs))
	for _, d := range defs {
		raw, e := json.Marshal(d.Manifest)
		if e != nil {
			return nil, ErrUnavailable
		}
		out = append(out, Definition{d.Manifest, digest(append(raw, []byte(BuildID())...))})
	}
	return out, nil
}
func definition(id string) (Definition, error) {
	all, e := Catalog()
	if e != nil {
		return Definition{}, e
	}
	for _, d := range all {
		if d.Manifest.ID == id {
			return d, nil
		}
	}
	return Definition{}, ErrNotFound
}
func scopes(d Definition) []string {
	out := make([]string, 0, len(d.Manifest.Permissions))
	for _, p := range d.Manifest.Permissions {
		out = append(out, p.Scope)
	}
	slices.Sort(out)
	return out
}
