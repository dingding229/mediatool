package appcli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourcesHaveNoFixedDeploymentOrRetiredEmbyTasks(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("repository root not found")
		}
		root = parent
	}
	for _, subtree := range []string{"internal", "cmd"} {
		err = filepath.WalkDir(filepath.Join(root, subtree), func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, retired := range []string{"185.99.135.205", "emby.refresh", "emby.confirm", "library.emby.refresh", "media-tool/internal/appcli/embylayout"} {
				if strings.Contains(string(raw), retired) {
					t.Errorf("retired reference %q in non-test source %s", retired, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetiredEmbyLayoutCommandCannotRun(t *testing.T) {
	for _, args := range [][]string{{"diagnose", "emby-layout"}, {"diagnose", "emby-layout", "--apply"}, {"diagnose", "emby-layout", "--help"}} {
		handled, err := Dispatch(args)
		if !handled || err == nil || !strings.Contains(err.Error(), "unknown application subcommand") {
			t.Fatal(args, handled, err)
		}
	}
}
