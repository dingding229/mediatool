// plugin-assets is build-only. Runtime has no compiler/downloader or exporter.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/dingding229/mediatool-plugin/registry"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type resource struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type inventory struct {
	HostMethods     []b.Method           `json:"hostMethods"`
	Pin             string               `json:"pin"`
	ContractVersion int                  `json:"contractVersion"`
	Manifests       []contracts.Manifest `json:"manifests"`
	Resources       []resource           `json:"resources"`
}

func main() {
	check := flag.Bool("check", false, "verify inventory without writes")
	flag.Parse()
	if e := export(*check); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func export(check bool) error {
	var module struct {
		Dir, Version string
		Replace      any
	}
	raw, e := exec.Command("go", "list", "-m", "-json", "github.com/dingding229/mediatool-plugin").Output()
	if e != nil {
		return e
	}
	if json.Unmarshal(raw, &module) != nil || module.Replace != nil || module.Dir == "" || module.Version == "" {
		return errors.New("exact plugin pin required; replace prohibited")
	}
	defs, e := registry.All()
	if e != nil {
		return e
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Manifest.ID < defs[j].Manifest.ID })
	out := inventory{HostMethods: b.Methods(), Pin: module.Version, ContractVersion: contracts.ContractVersion, Manifests: []contracts.Manifest{}, Resources: []resource{}}
	destination := "web/public/plugins"
	buildSources := "web/plugin-ui"
	if !check {
		if e = safeOutput(destination); e != nil {
			return e
		}
		if e = safeOutput(buildSources); e != nil {
			return e
		}
		if e = os.RemoveAll(buildSources); e != nil {
			return e
		}
		if e = os.RemoveAll(destination); e != nil {
			return e
		}
	}
	var totalBytes int64
	for _, d := range defs {
		out.Manifests = append(out.Manifests, d.Manifest)
		icon, e := exportIcon(module.Dir, d.Manifest.ID, d.Manifest.Icon, destination, check)
		if e != nil {
			return e
		}
		out.Resources = append(out.Resources, icon)
		source := filepath.Join(module.Dir, "plugins", d.Manifest.ID, "ui", "dist")
		if _, e = os.Stat(source); errors.Is(e, os.ErrNotExist) {
			continue
		} else if e != nil {
			return e
		}
		e = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("resource symlink prohibited")
			}
			if entry.IsDir() {
				return nil
			}
			info, e := entry.Info()
			if e != nil {
				return e
			}
			totalBytes += info.Size()
			if !info.Mode().IsRegular() || info.Size() > 4<<20 || totalBytes > 32<<20 || len(out.Resources) >= 1024 {
				return errors.New("resource type or size exceeds bound")
			}
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			rel, e := filepath.Rel(source, path)
			if e != nil {
				return e
			}
			name := filepath.ToSlash(filepath.Join("ui", "dist", rel))
			out.Resources = append(out.Resources, resource{d.Manifest.ID, name, hex.EncodeToString(sum[:])})
			if check {
				return nil
			}
			target := filepath.Join(destination, d.Manifest.ID, filepath.FromSlash(name))
			if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
				return e
			}
			if e = os.WriteFile(target, b, 0644); e != nil {
				return e
			}
			buildTarget := filepath.Join(buildSources, d.Manifest.ID, filepath.FromSlash(name))
			if e = os.MkdirAll(filepath.Dir(buildTarget), 0755); e != nil {
				return e
			}
			return os.WriteFile(buildTarget, b, 0644)
		})
		if e != nil {
			return e
		}
	}
	encoded, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		return e
	}
	encoded = append(encoded, '\n')
	target := "internal/modules/plugins/v2/library.json"
	if check {
		before, e := os.ReadFile(target)
		if e != nil {
			return e
		}
		if string(before) != string(encoded) {
			return errors.New("plugin resource inventory/pin mismatch; regenerate")
		}
	} else if e = os.WriteFile(target, encoded, 0644); e != nil {
		return e
	}
	fmt.Printf("Pinned inventory verified: %d definitions, %d resources\n", len(out.Manifests), len(out.Resources))
	return nil
}
func safeOutput(dir string) error {
	if (filepath.Clean(dir) != "web/public/plugins" && filepath.Clean(dir) != "web/plugin-ui") || strings.Contains(dir, "..") {
		return errors.New("invalid output root")
	}
	for _, p := range []string{"web", "web/public", dir} {
		st, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("output must be a directory, never a symlink")
		}
	}
	return nil
}
