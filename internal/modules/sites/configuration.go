package sites

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ErrConfig = errors.New("unsupported or invalid site configuration")
var ErrRequest = errors.New("site request failed; check login, network and compatibility")
var ErrAuthentication = errors.New("site authentication could not be verified")

type Map = map[string]any
type Config struct {
	HasHR                                   *bool
	ID, Name, Domain, API, Encoding, Schema string
	AdditionalDomains                       []string
	Requests                                map[string]Map
}

func Decode(raw, common []byte) (Config, error) { return decodeConfig(raw, common, false) }

// DecodeStored refuses legacy jsonb selector maps whose priority was lost.
// Re-import is explicit; it never silently fetches a different upstream ref.
func DecodeStored(raw, common []byte) (Config, error) { return decodeConfig(raw, common, true) }

func decodeConfig(raw, common []byte, stored bool) (Config, error) {
	root, err := decodeDefinition(raw, stored)
	if err != nil {
		return Config{}, err
	}
	base, err := decodeDefinition(common, stored)
	if err != nil {
		return Config{}, err
	}
	c := Config{ID: text(root["id"]), Name: text(root["name"]), Domain: text(root["domain"]), API: text(root["api"]), Schema: text(root["schema"]), Encoding: text(root["encoding"]), Requests: map[string]Map{}}
	if !regexp.MustCompile(`^[A-Za-z0-9_]{1,80}$`).MatchString(c.ID) || c.Domain == "" {
		return c, ErrConfig
	}
	if c.Schema == "" {
		c.Schema = text(root["reuse_schema"])
	}
	if value, ok := object(root["meta"])["has_hr"].(bool); ok {
		c.HasHR = &value
	}
	for _, v := range slice(root["additional_domain"]) {
		c.AdditionalDomains = append(c.AdditionalDomains, text(v))
	}
	for k, v := range object(base["requests"]) {
		if m, ok := v.(Map); ok {
			c.Requests[k] = clone(m)
		}
	}
	for name, v := range object(root["requests"]) {
		override := object(v)
		inherited := clone(c.Requests[name])
		if inherited == nil {
			inherited = Map{}
		}
		for key, disable := range object(override["disabled_extends"]) {
			if disable == true {
				delete(inherited, key)
			}
		}
		if parser := text(override["parser"]); parser != "" && parser != text(inherited["parser"]) {
			delete(inherited, "fields")
			delete(inherited, "list")
		}
		c.Requests[name] = merge(inherited, override)
	}
	return c, nil
}
func merge(base, override Map) Map {
	out := clone(base)
	if out == nil {
		out = Map{}
	}
	if _, ok := override["selector"]; ok {
		delete(out, "any")
		delete(out, "case")
	}
	if _, ok := override["case"]; ok {
		delete(out, "selector")
		delete(out, "any")
	}
	if _, ok := override["any"]; ok {
		delete(out, "selector")
		delete(out, "case")
	}
	for k, v := range override {
		if m, ok := v.(Map); ok {
			out[k] = merge(object(out[k]), m)
		} else {
			out[k] = v
		}
	}
	return out
}
func clone(in Map) Map {
	if in == nil {
		return nil
	}
	out := Map{}
	for k, v := range in {
		if m, ok := v.(Map); ok {
			out[k] = clone(m)
		} else {
			out[k] = v
		}
	}
	return out
}
func object(v any) Map  { m, _ := v.(map[string]any); return m }
func slice(v any) []any { s, _ := v.([]any); return s }
func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func blank(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	if a, ok := v.([]any); ok {
		return len(a) == 0
	}
	return false
}
func keys(m Map) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefinitionFingerprint binds the actual request/parser document loaded by a
// client, including its inherited rules and immutable source reference.
func DefinitionFingerprint(definition, common []byte, commit string) string {
	raw, _ := json.Marshal([]json.RawMessage{definition, common})
	digest := sha256.Sum256(append(append(raw, '\n'), []byte(commit+"\nparser-contract:containers-and-summary-seeding-v4")...))
	return hex.EncodeToString(digest[:])
}
