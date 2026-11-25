package assets

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	deferutil "media-tool/internal/platform/cleanup"
)

const pluginModule = "github.com/dingding229/mediatool-plugin"

//go:embed legacy_plugin_checksums.txt
var legacyPluginChecksums string

func legacyPluginSources(current, dir string) (map[string]string, error) {
	// Explicit module downloads may update go.sum. Use a separate temporary
	// module seeded with the historical checksums, never the application graph.
	work, err := os.MkdirTemp("", "mediatool-plugin-history-")
	if err != nil {
		return nil, err
	}
	defer deferutil.Do(func() error { return os.RemoveAll(work) }, "Remove only the temporary build-time module created by legacyPluginSources")
	if err = os.WriteFile(filepath.Join(work, "go.mod"), []byte("module mediatool-plugin-history\n"), 0600); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(work, "go.sum"), []byte(legacyPluginChecksums), 0600); err != nil {
		return nil, err
	}
	locked := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(legacyPluginChecksums), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != pluginModule || locked[f[1]] != "" {
			return nil, errors.New("invalid historical plugin checksum record")
		}
		locked[f[1]] = f[2]
	}
	sources := map[string]string{current: dir}
	for version, checksum := range locked {
		if strings.HasSuffix(version, "/go.mod") || version == current {
			continue
		}
		cmd := exec.Command("go", "mod", "download", "-json", pluginModule+"@"+version)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "GOWORK=off")
		data, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var mod struct{ Path, Dir, Version, Sum, GoModSum string }
		if json.Unmarshal(data, &mod) != nil || mod.Path != pluginModule || mod.Version != version || mod.Sum != checksum || locked[version+"/go.mod"] == "" || mod.GoModSum != locked[version+"/go.mod"] || mod.Dir == "" {
			return nil, errors.New("legacy content evidence does not match locked checksums")
		}
		sources[version] = mod.Dir
	}
	return sources, nil
}
