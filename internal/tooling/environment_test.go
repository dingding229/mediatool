package tooling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"media-tool/internal/platform/auth"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolationURLs(t *testing.T) {
	for _, tt := range []struct {
		db, r string
		ok    bool
	}{
		{"postgres://u@localhost/media_tool_test", "redis://localhost/0", true},
		{"postgresql://u@localhost/%6dedia_tool_test", "rediss://localhost/9?db=0", true},
		{"postgres://u@localhost/business", "redis://localhost/15", false},
		{"http://localhost/media_tool_test", "redis://localhost/15", false},
		{"postgres://u@localhost/media_tool_test", "http://localhost/15", false},
		{"postgres://u@localhost/media_tool_test#bad", "redis://localhost/15", false},
	} {
		t.Run(tt.db+tt.r, func(t *testing.T) {
			_, r, e := TestURLs(tt.db, tt.r)
			if (e == nil) != tt.ok {
				t.Fatal(e)
			}
			if e == nil {
				u, _ := url.Parse(r)
				if u.Path != "/15" || u.Query().Has("db") {
					t.Fatal(r)
				}
			}
		})
	}
}
func TestTestEnvironmentDoesNotCopyProductionSecrets(t *testing.T) {
	r := t.TempDir()
	os.WriteFile(filepath.Join(r, ".env"), []byte("DATABASE_URL=postgres://u@localhost/business\nAPP_SECRET_KEY=PRODUCTION-DO-NOT-COPY\nREDIS_URL=redis://localhost/0\n"), 0600)
	os.WriteFile(filepath.Join(r, ".env.test"), []byte("TEST_DATABASE_URL=postgres://u@localhost/media_tool_test\n"), 0600)
	v, e := TestEnvironment(r, func(string) string { return "" })
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(v["DATABASE_URL"], "business") || v["APP_SECRET_KEY"] == "PRODUCTION-DO-NOT-COPY" || len(v["APP_SECRET_KEY"]) != 64 {
		t.Fatal("test configuration copied production")
	}
	var b bytes.Buffer
	if e = ShellEnvironment(&b, v); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("bash", "-c", b.String()+`test "$DATABASE_URL" = "$TEST_DATABASE_URL" && test "$REDIS_URL" = "$TEST_REDIS_URL"`)
	if e = cmd.Run(); e != nil {
		t.Fatal(e)
	}
}
func TestTestEnvironmentFailsClosed(t *testing.T) {
	r := t.TempDir()
	if _, e := TestEnvironment(r, func(string) string { return "" }); e == nil {
		t.Fatal("missing configuration passed")
	}
	if _, e := TestEnvironment(r, func(k string) string {
		if k == "TEST_DATABASE_URL" {
			return "postgres://u@localhost/production"
		}
		return "redis://localhost/0"
	}); e == nil {
		t.Fatal("production passed")
	}
}
func TestTokensAndExclusiveConfiguration(t *testing.T) {
	a, _ := Token()
	b, _ := Token()
	if len(a) != 64 || a == b {
		t.Fatal("weak token")
	}
	r := t.TempDir()
	os.WriteFile(filepath.Join(r, ".env.example"), []byte("SESSION_SECRET_KEY=CHANGE_ME_SESSION_SECRET_KEY\nAPP_SECRET_KEY=CHANGE_ME_APP_SECRET_KEY\nSTRM_API_KEY=CHANGE_ME_STRM_API_KEY\n"), 0600)
	if e := initEnvironment(r, false); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(r, ".env"))
	if strings.Contains(string(before), "CHANGE_ME") {
		t.Fatal("uninitialized")
	}
	if e := initEnvironment(r, false); e == nil {
		t.Fatal("overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(r, ".env"))
	if !bytes.Equal(before, after) {
		t.Fatal("changed")
	}
	st, _ := os.Stat(filepath.Join(r, ".env"))
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestCIConfigurationIsDB15(t *testing.T) {
	r := t.TempDir()
	if e := initEnvironment(r, true); e != nil {
		t.Fatal(e)
	}
	v, e := ReadValues(filepath.Join(r, ".env.test"))
	if e != nil || v["TEST_REDIS_URL"] != "redis://127.0.0.1:6379/15" {
		t.Fatal(e, v)
	}
	if e = initEnvironment(r, true); e == nil {
		t.Fatal("overwrite")
	}
}
func TestCleanWebIsScopedAndDoesNotFollowSymlink(t *testing.T) {
	r := t.TempDir()
	a := filepath.Join(r, "internal/platform/webui/assets")
	os.MkdirAll(filepath.Join(a, "assets"), 0700)
	for _, n := range []string{".gitkeep", "sw.js", "index.html", "assets/build.js", "assets/build.css"} {
		os.WriteFile(filepath.Join(a, n), []byte("keep"), 0600)
	}
	outside := filepath.Join(r, "outside")
	os.WriteFile(outside, []byte("keep"), 0600)
	os.Symlink(outside, filepath.Join(a, "assets/link.js"))
	if e := cleanWeb(r); e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{".gitkeep", "sw.js", "assets/link.js"} {
		if _, e := os.Lstat(filepath.Join(a, n)); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat(filepath.Join(a, "index.html")); !os.IsNotExist(e) {
		t.Fatal("not cleaned")
	}
	raw, _ := os.ReadFile(outside)
	if string(raw) != "keep" {
		t.Fatal("escaped")
	}
}
func TestPortGuardRejectsArbitraryTargets(t *testing.T) {
	for _, s := range []string{"80", "65536", "host:18084", "0"} {
		if e := ports([]string{s}); e == nil {
			t.Fatal(s)
		}
	}
}
func project(t *testing.T) string {
	t.Helper()
	r, e := Root()
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func source(t *testing.T, p string) string {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(project(t), p))
	if e != nil {
		t.Fatal(e)
	}
	return string(raw)
}
func has(t *testing.T, s string, terms ...string) {
	t.Helper()
	for _, v := range terms {
		if !strings.Contains(s, v) {
			t.Fatalf("missing contract %q", v)
		}
	}
}
func TestNativeMediaAndPluginCIPolicies(t *testing.T) {
	w := source(t, ".github/workflows/build.yml")
	verify := strings.Split(strings.Split(w, "  verify:")[1], "  publish:")[0]
	has(t, verify, "timeout-minutes: 60", "sudo apt-get install -y ffmpeg", "go test -race -count=3 -v ./internal/modules/mediaprobe/...", `MEDIA_TOOL_REQUIRE_FFMPEG: "1"`, "bash scripts/test-go.sh", "bash scripts/test-plugin-kernel.sh", "go run ./cmd/media-tool-dev plugins check", "go run ./cmd/media-tool-dev plugins assets --check", "-ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/...")
	if strings.Index(verify, "sudo apt-get install") > strings.Index(verify, "bash scripts/test-go.sh") {
		t.Fatal("tools installed too late")
	}
	has(t, strings.Split(w, "  publish:")[1], "needs: verify")
	s := source(t, "scripts/test-go.sh")
	has(t, s, "command -v ffmpeg", "-race -count=1 -v", "-p 1", "-timeout=20m", "isolated_test")
	k := source(t, "scripts/test-plugin-kernel.sh")
	has(t, k, "CGO_ENABLED=0", "--reuid=65532", "--bounding-set=-all", "--no-new-privs", "MEDIA_TOOL_PLUGIN_BINARY", "x86_64")
	if strings.Contains(strings.ToLower(k), "qemu") {
		t.Fatal("emulated gate")
	}
	docker := source(t, "deploy/Dockerfile")
	has(t, docker, "COPY internal/platform/httpapi/testdata/contracts/ /src/internal/platform/httpapi/testdata/contracts/", "COPY internal/modules/plugins/v2/library.json /src/internal/modules/plugins/v2/library.json", "COPY --from=plugin-resources /src/web/public/")
	frontend := strings.Split(strings.Split(docker, " AS frontend")[1], " AS backend")[0]
	if strings.Index(frontend, "COPY internal/platform/httpapi/testdata/contracts/") > strings.Index(frontend, "RUN npm run build") {
		t.Fatal("late contracts")
	}
	has(t, source(t, "web/src/api/plugins.ts"), "internal/modules/plugins/v2/library.json")
}
func TestPluginNoRuntimeCompilerAndSharedReadyBarrier(t *testing.T) {
	r := project(t)
	for _, p := range []string{"plugin-sdk/go", "examples/plugins"} {
		if _, e := os.Stat(filepath.Join(r, p)); !os.IsNotExist(e) {
			t.Fatal("obsolete distribution", p)
		}
	}
	if strings.Contains(source(t, "go.mod"), "wazero") {
		t.Fatal("WASM runtime")
	}
	n := 0
	filepath.WalkDir(filepath.Join(r, "internal"), func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		s := string(b)
		if strings.HasSuffix(path, "_test.go") {
			if strings.Contains(s, "Supervisor{") && filepath.Base(path) != "task_limit_linux_test.go" {
				has(t, s, "testworker.Ready(")
				n++
			}
		} else if strings.Contains(path, "/plugins/v2/") {
			for _, bad := range []string{"go build", "go install", "go get", "http.Get(", `exec.Command("sh"`} {
				if strings.Contains(s, bad) {
					t.Fatal(path, bad)
				}
			}
		}
		return nil
	})
	if n < 8 {
		t.Fatal("insufficient real-worker suites", n)
	}
	has(t, source(t, "internal/platform/testworker/ready.go"), "runtime_state", "generation", "grant_revision", "lease_until", "Extensions.AuthProviders", "probe(bounded)")
}
func TestBrowserStartupNodeHarness(t *testing.T) {
	cmd := exec.Command("node", filepath.Join(project(t), "scripts/browser_startup_guard_harness.cjs"), filepath.Join(project(t), "internal/modules/emby/browser_startup_guard.js"))
	out, e := cmd.Output()
	if e != nil {
		t.Fatal("owned startup harness", e)
	}
	var v struct {
		Cases int `json:"cases"`
	}
	if e = json.Unmarshal(out, &v); e != nil || v.Cases < 30 {
		t.Fatal("guard cases", e, string(out))
	}
}
func TestTestLockSerializesSeparateProcesses(t *testing.T) {
	r := t.TempDir()
	marker := filepath.Join(r, "active")
	command := fmt.Sprintf(`if ! mkdir '%s';then exit 9;fi;sleep .2;rmdir '%s'`, marker, marker)
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- locked(r, []string{"bash", "-c", command}) }()
	}
	for range 2 {
		if e := <-errs; e != nil {
			t.Fatal("lock overlapped", e)
		}
	}
}

func TestFixturePasswordIsNotAnApplicationKeyAndKeepsProductPolicy(t *testing.T) {
	a, e := FixturePassword()
	if e != nil {
		t.Fatal(e)
	}
	b, e := FixturePassword()
	if e != nil || a == b || len(a) != 16 {
		t.Fatal("invalid fixture entropy/length")
	}
	if e = auth.ValidatePassword(a); e != nil {
		t.Fatal("fixture violates existing password policy", e)
	}
	key, _ := Token()
	if len(key) != 64 {
		t.Fatal("application keys were shortened")
	}
}
