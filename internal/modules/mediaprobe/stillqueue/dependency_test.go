package stillqueue

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStateRepositoryDoesNotExecuteOrTouchFiles(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("workspace unavailable")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(here), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range tree.Imports {
			name := strings.Trim(imp.Path.Value, `"`)
			if name == "os" || name == "os/exec" || name == "net/http" || strings.Contains(name, "mediaprobe/ffx") || strings.Contains(name, "mediaprobe/probequeue") {
				t.Fatal("A repository crossed execution boundary", name)
			}
		}
	}
}
