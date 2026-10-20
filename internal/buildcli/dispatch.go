// Package buildcli is linked only into development tooling, not the runtime app.
package buildcli

import (
	"media-tool/internal/buildcli/assets"
	"media-tool/internal/buildcli/check"
	"media-tool/internal/buildcli/siteaudit"
	"media-tool/internal/platform/cliprocess"
)

func Dispatch(args []string) bool {
	if len(args) < 2 {
		return false
	}
	key := args[0] + " " + args[1]
	m := map[string]func(){"plugins assets": assets.Main, "plugins check": check.Main, "sites audit": siteaudit.Main}
	if run, ok := m[key]; ok {
		cliprocess.Invoke(key, args[2:], run)
		return true
	}
	return false
}
