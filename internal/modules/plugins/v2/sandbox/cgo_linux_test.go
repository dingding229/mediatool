//go:build linux && cgo

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("PLUGIN_CGO_LANDLOCK_HELPER") == "1" {
		active, e := Landlock(os.Args[1], os.Args[2], nil)
		if e != nil {
			os.Exit(2)
		}
		if active {
			fmt.Println("active")
		} else {
			fmt.Println("unavailable")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// CGO_ENABLED and actual runtime/cgo linkage are different. Run in a child so a
// pure-Go test with CGO_ENABLED=1 cannot accidentally Landlock the test runner.
// Neither outcome substitutes for the mandatory CGO_ENABLED=0 release gate.
func TestLandlockCgoOrRaceWarningMode(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	tmp := filepath.Join(root, "tmp")
	for _, dir := range []string{data, tmp} {
		if e := os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, data, tmp)
	cmd.Env = []string{"PLUGIN_CGO_LANDLOCK_HELPER=1"}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	mode := strings.TrimSpace(string(out))
	if mode != "active" && mode != "unavailable" {
		t.Fatal(mode)
	}
	t.Log("CGO/race helper mode:", mode, "; static release isolation is a separate mandatory gate")
}
