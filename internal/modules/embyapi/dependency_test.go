package embyapi

import (
	"context"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProtocolPackageHasNoProbeFFmpegOrDatabaseCalls(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate package")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(here), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		node, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range node.Imports {
			value := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(value, "mediaprobe/ffx") || strings.Contains(value, "ffmpeg") || strings.Contains(value, "ffprobe") || value == "os/exec" || value == "database/sql" || strings.HasPrefix(value, "github.com/jackc/pgx") || strings.HasPrefix(value, "github.com/redis/go-redis") || value == "media-tool/internal/platform/database" {
				t.Fatalf("protocol layer imports forbidden dependency %s", value)
			}
		}
	}
	// Also inspect transitive dependencies; an adapter must not hide ffx imports.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", ".")
	command.Dir = filepath.Dir(here)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("dependency graph check failed: %v %s", err, output)
	}
	for _, name := range strings.Fields(string(output)) {
		if strings.Contains(name, "mediaprobe/ffx") || strings.Contains(strings.ToLower(name), "ffmpeg") || strings.Contains(strings.ToLower(name), "ffprobe") || strings.HasPrefix(name, "github.com/jackc/pgx") || strings.HasPrefix(name, "github.com/redis/go-redis") || name == "os/exec" {
			t.Fatalf("forbidden transitive dependency %s", name)
		}
	}
}
