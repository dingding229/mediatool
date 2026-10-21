// Package v2 implements built-in plugin instances. It never imports the WASM runner.
package v2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/registry"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"runtime/debug"
	"slices"
	"sync"
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
	Manifest       contracts.Manifest `json:"manifest"`
	InstalledCount *int               `json:"installedCount,omitempty"`
	Fingerprint    string             `json:"fingerprint"`
}
type Instance struct {
	ID           string       `json:"id"`
	PluginID     string       `json:"pluginId"`
	Desired      Desired      `json:"desiredState"`
	State        State        `json:"state"`
	Revision     int64        `json:"revision"`
	Reason       Reason       `json:"reason"`
	Metadata     *Metadata    `json:"metadata,omitempty"`
	Availability Availability `json:"availability,omitempty"`
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
	var pin struct {
		Pin string `json:"pin"`
	}
	if json.Unmarshal(libraryInventory, &pin) == nil {
		return pin.Pin
	}
	return ""
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Pinned compiled metadata is immutable; do not rehash the entire inventory per
// host RPC. Catalog still returns fresh manifests so callers cannot mutate it.
var contentFingerprints = sync.OnceValues(func() (map[string]string, error) {
	defs, e := registry.All()
	if e != nil {
		return nil, ErrUnavailable
	}
	var inventory struct {
		ContentHashes map[string]string `json:"contentHashes"`
	}
	if json.Unmarshal(libraryInventory, &inventory) != nil {
		return nil, ErrUnavailable
	}
	out := map[string]string{}
	for _, d := range defs {
		hash := inventory.ContentHashes[d.Manifest.ID]
		if len(hash) != 64 {
			return nil, ErrUnavailable
		}
		raw, e := canonicalManifest(d.Manifest)
		if e != nil {
			return nil, ErrUnavailable
		}
		out[d.Manifest.ID] = "content-v1:" + digest(append(raw, []byte(hash)...))
	}
	return out, nil
})

// Only immutable compiled metadata is memoized. Public/internal callers receive
// detached graphs; live instance grants, generation and revision remain PG reads.
type metadataSnapshot struct {
	All  json.RawMessage
	ByID map[string]json.RawMessage
}

var compiledMetadata = sync.OnceValues(func() (metadataSnapshot, error) {
	out := metadataSnapshot{ByID: map[string]json.RawMessage{}}
	defs, err := registry.All()
	if err != nil {
		return out, ErrUnavailable
	}
	fingerprints, err := contentFingerprints()
	if err != nil {
		return out, err
	}
	list := make([]Definition, 0, len(defs))
	for _, d := range defs {
		v := Definition{Manifest: d.Manifest, Fingerprint: fingerprints[d.Manifest.ID]}
		raw, err := json.Marshal(v)
		if err != nil {
			return out, ErrUnavailable
		}
		out.ByID[d.Manifest.ID] = raw
		list = append(list, v)
	}
	out.All, err = json.Marshal(list)
	return out, err
})

func Catalog() ([]Definition, error) {
	cached, err := compiledMetadata()
	if err != nil {
		return nil, err
	}
	var out []Definition
	if err = json.Unmarshal(cached.All, &out); err != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
func definition(id string) (Definition, error) {
	cached, err := compiledMetadata()
	if err != nil {
		return Definition{}, err
	}
	raw, ok := cached.ByID[id]
	if !ok {
		return Definition{}, ErrNotFound
	}
	var out Definition
	if err = json.Unmarshal(raw, &out); err != nil {
		return Definition{}, ErrUnavailable
	}
	return out, nil
}

func scopes(d Definition) []string {
	out := make([]string, 0, len(d.Manifest.Permissions))
	for _, p := range d.Manifest.Permissions {
		out = append(out, p.Scope)
	}
	slices.Sort(out)
	return out
}

func canonicalManifest(m contracts.Manifest) ([]byte, error) {
	raw, e := json.Marshal(m)
	if e != nil {
		return nil, e
	}
	var v any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if e = decoder.Decode(&v); e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
