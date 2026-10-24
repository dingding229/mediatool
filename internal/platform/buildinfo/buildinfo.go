// Package buildinfo exposes build-time provenance; release numbers come from Git tags.
package buildinfo

import "runtime/debug"

const Schema int64 = 82
const MinimumSchema int64 = 79

// Official builds inject these three values. The sentinel is intentionally not
// a release version: unversioned go run/test must never claim to be a release.
var Version = "0.0.0-dev+unknown"
var Commit = ""
var BuildTime = ""

// Revision is the existing internal maintenance/audit name for the same commit.
var Revision string

func init() {
	if Commit == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, v := range info.Settings {
				if v.Key == "vcs.revision" {
					Commit = v.Value
				}
			}
		}
	}
	Revision = Commit
}

type Info struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"buildTime"`
	Schema        int64  `json:"schema"`
	MinimumSchema int64  `json:"minimumSchema"`
	PluginPin     string `json:"pluginPin"`
}

func Current() Info {
	v := Info{Version: Version, Commit: Commit, BuildTime: BuildTime, Schema: Schema, MinimumSchema: MinimumSchema, PluginPin: "unknown"}
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, d := range b.Deps {
			if d.Path == "github.com/dingding229/mediatool-plugin" {
				v.PluginPin = d.Version
				if d.Replace != nil {
					v.PluginPin = "unreleased-replacement"
				}
			}
		}
	}
	return v
}
