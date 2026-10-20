package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellRunnerIsolationAndFixedRaceArguments(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "media-tool-dev")
	build := exec.Command("go", "build", "-o", bin, "./cmd/media-tool-dev")
	build.Dir = project(t)
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	for _, tt := range []struct {
		name, db, platform string
		args               []string
		media, fail        bool
	}{
		{name: "serial", db: "media_tool_test", platform: "Darwin"},
		{name: "business-refused", db: "business", platform: "Darwin", fail: true},
		{name: "packages", db: "media_tool_test", platform: "Darwin", args: []string{"./internal/platform/secrets"}},
		{name: "parallel-override", db: "media_tool_test", platform: "Darwin", args: []string{"-p=8"}, fail: true},
		{name: "real-media", db: "media_tool_test", platform: "Darwin", media: true},
		{name: "native-static-worker", db: "media_tool_test", platform: "Linux"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := t.TempDir()
			os.MkdirAll(filepath.Join(r, "scripts"), 0700)
			os.MkdirAll(filepath.Join(r, "fake"), 0700)
			for _, n := range []string{"test-go.sh", "test-common.sh"} {
				raw, _ := os.ReadFile(filepath.Join(project(t), "scripts", n))
				os.WriteFile(filepath.Join(r, "scripts", n), raw, 0700)
			}
			os.WriteFile(filepath.Join(r, "go.mod"), []byte("module media-tool\n"), 0600)
			os.WriteFile(filepath.Join(r, ".env.test"), []byte("TEST_DATABASE_URL=postgres://fixture@localhost/"+tt.db+"\nTEST_REDIS_URL=redis://localhost/0\n"), 0600)
			record := filepath.Join(r, "commands")
			fake := `#!/bin/bash
printf '%s|%s|%s|%s\n' "$*" "$DATABASE_URL" "$REDIS_URL" "${CGO_ENABLED:-}" >> "$COMMAND_RECORD"
if [[ "$1" == build ]];then while [[ "$#" -gt 0 ]];do if [[ "$1" == -o ]];then shift;printf '#!/bin/sh\nexit 0\n' > "$1";chmod 700 "$1";break;fi;shift;done;fi
`
			os.WriteFile(filepath.Join(r, "fake/go"), []byte(fake), 0700)
			os.WriteFile(filepath.Join(r, "fake/uname"), []byte("#!/bin/sh\necho "+tt.platform+"\n"), 0700)
			cmd := exec.Command("bash", append([]string{"scripts/test-go.sh"}, tt.args...)...)
			cmd.Dir = r
			// Deliberately construct the environment: an ambient real-media switch cannot contaminate cases.
			cmd.Env = []string{"PATH=" + filepath.Join(r, "fake") + ":" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "MEDIA_TOOL_DEV_BIN=" + bin, "COMMAND_RECORD=" + record, "TMPDIR=" + os.TempDir()}
			if tt.media {
				cmd.Env = append(cmd.Env, "MEDIA_TOOL_REQUIRE_FFMPEG=1")
			}
			out, e := cmd.CombinedOutput()
			if (e != nil) != tt.fail {
				t.Fatal(e, string(out))
			}
			raw, _ := os.ReadFile(record)
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if tt.fail {
				if len(raw) != 0 {
					t.Fatal("guard ran Go", string(raw))
				}
				return
			}
			for _, l := range lines {
				has(t, l, "/media_tool_test|redis://localhost/15|")
			}
			has(t, string(raw), "run ./cmd/media-tool migrate -test-only -action up -cleanup-test-plugins")
			expected := "test -p 1 -race -count=1 "
			if tt.media {
				expected += "-v "
				has(t, string(out), "Real media tests REQUIRED")
			}
			expected += "-timeout=20m "
			if len(tt.args) > 0 {
				expected += tt.args[0]
			} else {
				expected += "./..."
			}
			has(t, string(raw), expected)
			if tt.platform == "Linux" {
				if len(lines) != 3 {
					t.Fatal(lines)
				}
				has(t, lines[1], "build -trimpath -o", "./cmd/media-tool", "|0")
				fields := strings.Fields(strings.Split(lines[1], "|")[0])
				for i, a := range fields {
					if a == "-o" {
						if _, e = os.Stat(filepath.Dir(fields[i+1])); !os.IsNotExist(e) {
							t.Fatal("worker residue")
						}
					}
				}
				has(t, string(out), "Real static app plugin-worker REQUIRED")
			}
		})
	}
}
func TestNativeExecAdapterRefusesOutsideDedicatedCI(t *testing.T) {
	for _, name := range []string{"plugins.test", "v2.test", "httpapi.test", "acquisition.test"} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), name)
			os.WriteFile(p, []byte("#!/bin/sh\necho SHOULD_NOT_RUN\n"), 0700)
			cmd := exec.Command("bash", filepath.Join(project(t), "scripts/test-go-plugin-exec.sh"), p)
			cmd.Env = append(os.Environ(), "GITHUB_ACTIONS=false")
			out, e := cmd.CombinedOutput()
			if e == nil || strings.Contains(string(out), "SHOULD_NOT_RUN") {
				t.Fatal("unsafe execution", e, string(out))
			}
			has(t, string(out), "Dedicated native CI identity required")
		})
	}
}
