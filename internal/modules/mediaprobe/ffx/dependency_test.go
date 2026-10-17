package ffx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const probePackage = "media-tool/internal/modules/mediaprobe"

func allowedImporter(name string) bool {
	return name == probePackage || strings.HasPrefix(name, probePackage+"/")
}
func TestOnlyMediaProbePackagesMayImportFFX(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate workspace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = filepath.Clean(filepath.Join(filepath.Dir(here), "../../../.."))
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("dependency graph", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	count := 0
	for {
		var pkg struct {
			ImportPath                         string
			Imports, TestImports, XTestImports []string
		}
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		for _, deps := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
			for _, dep := range deps {
				if (dep == probePackage+"/ffx" || strings.HasPrefix(dep, probePackage+"/ffx/")) && !allowedImporter(pkg.ImportPath) {
					t.Errorf("%s imports forbidden process boundary %s", pkg.ImportPath, dep)
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no packages checked")
	}
}
func TestImporterBoundaryIsPathScoped(t *testing.T) {
	for _, name := range []string{probePackage, probePackage + "/ffx", probePackage + "/worker"} {
		if !allowedImporter(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"media-tool/internal/modules/embyapi", "media-tool/internal/modules/playback", probePackage + "evil"} {
		if allowedImporter(name) {
			t.Fatal(name)
		}
	}
}
