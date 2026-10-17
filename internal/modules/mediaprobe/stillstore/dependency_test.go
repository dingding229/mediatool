package stillstore

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileStoreDoesNotInvokeProbeOrWireBusinessState(t *testing.T) {
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
			if name == "os/exec" || name == "net/http" || strings.Contains(name, "/mediaprobe/ffx") || strings.Contains(name, "/mediaprobe/linksource") || strings.Contains(name, "pgx") || strings.Contains(name, "redis") {
				t.Fatal("file store crossed boundary", name)
			}
		}
	}
	// Both platform implementations must use an atomic, no-replace primitive.
	for file, flag := range map[string]string{"rename_linux.go": "unix.RENAME_NOREPLACE", "rename_darwin.go": "unix.RENAME_EXCL"} {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), flag) || strings.Contains(string(raw), "os.Rename") {
			t.Fatal("unsafe rename fallback", file)
		}
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../../../.."))
	dockerfile, err := os.ReadFile(filepath.Join(root, "deploy", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	_, runtimePart, ok := strings.Cut(string(dockerfile), " AS runtime\n")
	if !ok {
		t.Fatal("runtime stage missing")
	}
	if !strings.Contains(runtimePart, "mkdir -p /data/strm /data/downloads /data/files /data/native-media/stills && chown -R app:app /data") || !strings.Contains(runtimePart, "USER 65532:65532") {
		t.Fatal("runtime still directory/owner missing")
	}
}
