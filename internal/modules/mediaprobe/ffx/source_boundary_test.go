package ffx

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func scanFFXImports(root fs.FS) ([]string, error) {
	var violations []string
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && name != "." || d.Name() == "node_modules" || d.Name() == "data" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		raw, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, raw, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			value, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if value == probePackage+"/ffx" || strings.HasPrefix(value, probePackage+"/ffx/") {
				importer := "media-tool/" + path.Dir(name)
				if !allowedImporter(importer) {
					violations = append(violations, name)
				}
			}
		}
		return nil
	})
	return violations, err
}
func TestAllNonTestSourcesRespectFFXBoundary(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("workspace path unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../../../.."))
	violations, err := scanFFXImports(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatal("forbidden ffx imports", violations)
	}
}
func TestSourceGuardChecksOtherBuildTagsAndIgnoresTests(t *testing.T) {
	files := fstest.MapFS{
		"internal/modules/playback/runner_windows.go": {Data: []byte("//go:build windows\npackage playback\nimport _ \"" + probePackage + "/ffx\"\n")},
		"internal/modules/mediaprobe/worker.go":       {Data: []byte("package mediaprobe\nimport _ \"" + probePackage + "/ffx\"\n")},
		"internal/modules/embyapi/example_test.go":    {Data: []byte("package embyapi\nimport _ \"" + probePackage + "/ffx\"\n")},
	}
	violations, err := scanFFXImports(files)
	if err != nil || len(violations) != 1 || !strings.Contains(violations[0], "runner_windows.go") {
		t.Fatal(violations, err)
	}
}
