// Package appcli dispatches application functions inside the same app binary.
package appcli

import (
	"fmt"
	"media-tool/internal/appcli/adminreset"
	"media-tool/internal/appcli/backupverify"
	"media-tool/internal/appcli/cloudevents"
	"media-tool/internal/appcli/cloudsecurity"
	"media-tool/internal/appcli/cloudua"
	"media-tool/internal/appcli/cloudupload"
	"media-tool/internal/appcli/embylayout"
	"media-tool/internal/appcli/integrations"
	"media-tool/internal/appcli/migrate"
	"media-tool/internal/appcli/ops"
	"media-tool/internal/appcli/playback"
	"media-tool/internal/appcli/provider"
	"media-tool/internal/appcli/rekey"
	"media-tool/internal/appcli/sitesync"
	"media-tool/internal/appcli/updater"
	"media-tool/internal/platform/cliprocess"
)

func Dispatch(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	group := args[0]
	key := group
	rest := args[1:]
	if group == "admin" || group == "backup" || group == "diagnose" || group == "secrets" || group == "sites" {
		if len(rest) == 0 {
			return true, fmt.Errorf("%s requires a subcommand; see docs/CLI_REFERENCE.md", group)
		}
		key += " " + rest[0]
		rest = rest[1:]
	}
	if len(rest) == 1 && (rest[0] == "--help" || rest[0] == "-h") && (key == "ops" || key == "diagnose playback" || key == "diagnose integrations") {
		fmt.Println(key + ": owner-only process command; see docs/CLI_REFERENCE.md. No operation performed.")
		return true, nil
	}
	commands := map[string]func(){"admin reset": adminreset.Main, "backup verify": backupverify.Main, "diagnose cloud-ua": cloudua.Main, "diagnose cloud-events": cloudevents.Main, "diagnose cloud-upload": cloudupload.Main, "diagnose cloud-security": cloudsecurity.Main, "diagnose emby-layout": embylayout.Main, "diagnose integrations": integrations.Main, "diagnose playback": playback.Main, "diagnose provider": provider.Main, "update-policy-check": updater.Policy, "updater": updater.Main, "migrate": migrate.Main, "ops": ops.Main, "secrets rekey": rekey.Main, "sites sync": sitesync.Main}
	if main, ok := commands[key]; ok {
		cliprocess.Invoke(key, rest, main)
		return true, nil
	}
	if key != group {
		return true, fmt.Errorf("unknown application subcommand %q", key)
	}
	return false, nil
}
