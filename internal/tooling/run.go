// Package tooling provides local build/test commands. It never uses production
// database defaults or invokes a real-media/owner-password-reset fixture.
package tooling

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/buildcli"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func Root() (string, error) {
	p, e := os.Getwd()
	if e != nil {
		return "", e
	}
	for {
		if b, e := os.ReadFile(filepath.Join(p, "go.mod")); e == nil && strings.HasPrefix(string(b), "module media-tool\n") {
			return p, nil
		}
		next := filepath.Dir(p)
		if next == p {
			return "", errors.New("media-tool project root required")
		}
		p = next
	}
}
func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: media-tool-dev <test-lock|test-env|token|ports|fixture-emby|clean-web|init-env|ci-config>")
	}
	if len(args) >= 2 && args[0] == "plugins" && args[1] == "matrix" {
		args = args[1:]
	}
	root, e := Root()
	if e != nil {
		return e
	}
	switch args[0] {
	case "build-meta":
		return buildMetaCommand(root, args[1:])
	case "release-notes":
		return releaseNotesCommand(root, args[1:])
	case "test-lock":
		return locked(root, args[1:])
	case "test-env":
		if len(args) != 1 {
			return errors.New("test-env accepts no arguments")
		}
		v, e := TestEnvironment(root, os.Getenv)
		if e != nil {
			return e
		}
		return ShellEnvironment(os.Stdout, v)
	case "fixture-password":
		if len(args) != 1 {
			return errors.New("fixture-password accepts no arguments")
		}
		v, e := FixturePassword()
		if e != nil {
			return e
		}
		_, e = fmt.Fprintln(os.Stdout, v)
		return e
	case "token":
		if len(args) != 1 {
			return errors.New("token accepts no arguments")
		}
		s, e := Token()
		if e != nil {
			return e
		}
		_, e = fmt.Fprintln(os.Stdout, s)
		return e
	case "ports":
		return ports(args[1:])
	case "fixture-emby":
		return embyFixture()
	case "clean-web":
		return cleanWeb(root)
	case "init-env":
		return initEnvironment(root, false)
	case "ci-config":
		return initEnvironment(root, true)
	case "matrix":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--check") {
			return errors.New("matrix [--check]")
		}
		return matrix(root, len(args) == 2)
	default:
		return runExtra(root, args)
	}
}

// Extended build commands are kept separate from test infrastructure.
func runExtra(_ string, args []string) error {
	if buildcli.Dispatch(args) {
		return nil
	}
	return errors.New("unknown development command; see docs/CLI_REFERENCE.md")
}

func Token() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func locked(root string, args []string) error {
	if len(args) == 0 {
		return errors.New("test-lock requires a command")
	}
	if e := os.MkdirAll(filepath.Join(root, "data"), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(root, "data/.isolated-test.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer func() { _ = f.Close() }()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "MEDIA_TOOL_TEST_LOCKED=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{f}
	return cmd.Run()
}
func ports(args []string) error {
	if len(args) == 0 {
		return errors.New("ports requires loopback port numbers")
	}
	for _, s := range args {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1024 || n > 65535 {
			return errors.New("invalid isolated port")
		}
		l, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", s))
		if e != nil {
			return errors.New("isolated test ports busy; no existing process will be stopped")
		}
		if e = l.Close(); e != nil {
			return e
		}
	}
	return nil
}
func embyFixture() error {
	s := &http.Server{Addr: "127.0.0.1:18086", ReadHeaderTimeout: 5e9, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		if r.URL.Path != "/Users" || r.Header.Get("X-Emby-Token") != "PRIVATE-EMBY-ECHO-FIXTURE" {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "owned fixture authentication required"})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"Id": "owned-fixture-user", "Name": "Owned E2E fixture"}})
	})}
	return s.ListenAndServe()
}
func cleanWeb(root string) error {
	base := filepath.Join(root, "internal/platform/webui/assets")
	for _, pattern := range []string{"assets/*.js", "assets/*.css", "assets/*.js.gz", "assets/*.css.gz", "index.html"} {
		paths, e := filepath.Glob(filepath.Join(base, pattern))
		if e != nil {
			return e
		}
		for _, p := range paths {
			st, e := os.Lstat(p)
			if e != nil {
				return e
			}
			if st.Mode().IsRegular() {
				if e = os.Remove(p); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func ShellEnvironment(w io.Writer, values map[string]string) error {
	for _, k := range []string{"DATABASE_URL", "TEST_DATABASE_URL", "REDIS_URL", "TEST_REDIS_URL", "APP_ENV", "APP_PUBLIC_URL", "APP_SECRET_KEY", "SESSION_SECRET_KEY", "STRM_API_KEY"} {
		if v, ok := values[k]; ok {
			if _, e := fmt.Fprintf(w, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", "'\"'\"'")); e != nil {
				return e
			}
		}
	}
	return nil
}

// FixturePassword retains the original 16-character synthetic E2E password;
// application/session/STRM keys must still use the separate full-length Token.
func FixturePassword() (string, error) {
	v, e := Token()
	if e != nil {
		return "", e
	}
	return "E2E-" + v[:12], nil
}
