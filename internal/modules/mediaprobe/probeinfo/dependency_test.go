package probeinfo

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const purePackage = "media-tool/internal/modules/mediaprobe/probeinfo"

func boundaryViolations(root fs.FS) ([]string, error) {
	var bad []string
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "data" || strings.HasPrefix(d.Name(), ".") && name != "." {
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
			dep, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			catalog := strings.HasPrefix(name, "internal/modules/nativelibrary/catalog/")
			pure := strings.HasPrefix(name, "internal/modules/mediaprobe/probeinfo/")
			if catalog && strings.HasPrefix(dep, "media-tool/internal/modules/mediaprobe") && dep != purePackage || pure && (strings.HasPrefix(dep, "media-tool/") || dep == "os" || dep == "os/exec" || dep == "net" || dep == "net/http" || dep == "database/sql" || strings.HasPrefix(dep, "github.com/jackc/pgx") || strings.HasPrefix(dep, "github.com/redis")) {
				bad = append(bad, name+":"+dep)
			}
		}
		return nil
	})
	return bad, err
}
func TestPureResultAndCatalogBoundary(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../../../.."))
	bad, err := boundaryViolations(os.DirFS(root))
	if err != nil || len(bad) != 0 {
		t.Fatal(bad, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	for _, pkg := range []string{purePackage, "media-tool/internal/modules/nativelibrary/catalog"} {
		cmd := exec.CommandContext(ctx, "go", "list", "-deps", pkg)
		cmd.Dir = root
		raw, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, dep := range strings.Fields(string(raw)) {
			if strings.Contains(dep, "mediaprobe/") && dep != purePackage && dep != pkg {
				t.Fatal("forbidden transitive dependency", dep)
			}
			if pkg == purePackage && (dep == "os/exec" || dep == "net/http" || strings.Contains(dep, "pgx") || strings.Contains(dep, "redis") || dep == "database/sql") {
				t.Fatal("result package is not pure", dep)
			}
		}
	}
}
func TestGuardIncludesOtherPlatforms(t *testing.T) {
	files := fstest.MapFS{
		"internal/modules/nativelibrary/catalog/w_windows.go": {Data: []byte("//go:build windows\npackage catalog\nimport _ \"media-tool/internal/modules/mediaprobe/probequeue\"\n")},
		"internal/modules/nativelibrary/catalog/ok.go":        {Data: []byte("package catalog\nimport _ \"" + purePackage + "\"\n")},
		"internal/modules/mediaprobe/probeinfo/bad.go":        {Data: []byte("package probeinfo\nimport _ \"media-tool/internal/modules/mediaprobe/ffx\"\n")},
	}
	bad, err := boundaryViolations(files)
	if err != nil || len(bad) != 2 {
		t.Fatal(bad, err)
	}
}
