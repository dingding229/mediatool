package appcli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllApplicationLeavesHaveOfflineHelpAndOpsInfo(t *testing.T) {
	root, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e = os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("root")
		}
		root = next
	}
	bin := filepath.Join(t.TempDir(), "media-tool")
	build := exec.Command("go", "build", "-o", bin, "./cmd/media-tool")
	build.Dir = root
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatal(e, string(b))
	}
	for _, leaf := range [][]string{{"migrate"}, {"admin", "reset"}, {"backup", "verify"}, {"ops"}, {"secrets", "rekey"}, {"sites", "sync"}, {"diagnose", "cloud-ua"}, {"diagnose", "cloud-security"}, {"diagnose", "cloud-events"}, {"diagnose", "cloud-upload"}, {"diagnose", "emby-layout"}, {"diagnose", "integrations"}, {"diagnose", "playback"}, {"diagnose", "provider"}, {"diagnose", "session"}} {
		t.Run(strings.Join(leaf, "/"), func(t *testing.T) {
			cmd := exec.Command(bin, append(leaf, "--help")...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
			if b, e := cmd.CombinedOutput(); e != nil {
				t.Fatal("help touched configuration", e, string(b))
			}
		})
	}
	cmd := exec.Command(bin, "ops", "info")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	b, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Schema  int64
		Version string
	}
	if e = json.Unmarshal(b, &v); e != nil || v.Schema != 65 || v.Version == "" {
		t.Fatal("info needs no environment", e, string(b))
	}
	cmd = exec.Command(bin, "not-a-command")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	if b, e := cmd.CombinedOutput(); e == nil || !strings.Contains(string(b), "unknown command") {
		t.Fatal("unknown command started app", e, string(b))
	}
}
