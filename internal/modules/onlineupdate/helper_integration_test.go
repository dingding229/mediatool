package onlineupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"media-tool/internal/modules/updatecheck"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const helperFixtureImage = "dingding229/mediatool@sha256:f8d74ba13ef7fcf0411551b4f0d5def5a2eea325a86637c6bc4ec063849aa514"
const helperFixtureOld = "dingding229/mediatool@sha256:e458207bb03f850eaf80f828fad1460c1e09f2456903af413b0fc8fb5fb27a3b"

// Runs INSIDE the real temporary Docker container. Only the public Release/Hub
// HTTP replies are fixtures. This calls the exported CLI entrypoint, not Steps,
// runner.pg or a runner literal. Every Docker/PG/Compose/health operation is real.
func TestDockerHelperChild(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_HELPER_CHILD") != "1" {
		t.Skip("Docker child only")
	}
	id := os.Getenv("MEDIA_TOOL_HELPER_ID")
	j, e := load(Root, id)
	if e != nil {
		t.Fatal(e)
	}
	p, err := readPlan(Root, id)
	if err != nil {
		t.Fatal(err)
	}
	probe := newDocker()
	probe.self = p.Self
	actual, err := probe.inspect(t.Context(), p.Self.ID)
	if err != nil {
		t.Fatal("real inspect", err)
	}
	t.Logf("identity id=%t name=%t image=%t user=%t project=%t fingerprint=%t", actual.ID == p.Self.ID, actual.Name == p.Self.Name, fmtString(actual.Config["Image"]) == j.FromImage, actual.Config["User"] == "65532:65532", labels(actual)["com.docker.compose.project"] == probe.project(), configurationProof(actual) == p.ConfigurationProof)
	body, e := updatecheck.ReleaseBody("isolated test release", j.Target.Metadata)
	if e != nil {
		t.Fatal(e)
	}
	transport := http.DefaultTransport
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var v any
		switch r.URL.String() {
		case "https://api.github.com/repos/dingding229/mediatool/releases/latest":
			v = map[string]any{"tag_name": j.Target.Tag, "body": body, "published_at": time.Now().Add(-time.Hour)}
		case "https://hub.docker.com/v2/namespaces/dingding229/repositories/mediatool/tags/" + j.Target.Tag:
			v = map[string]any{"name": j.Target.Tag, "digest": j.Target.Digest, "images": []any{map[string]any{"architecture": "amd64", "os": "linux", "digest": j.Target.Digest}}}
		default:
			return transport.RoundTrip(r) // real readyz on the new app's bridge IP.
		}
		calls++
		b, _ := json.Marshal(v)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}, nil
	})
	defer func() { http.DefaultTransport = transport }()
	e = RunUpdater(t.Context(), id)
	j, readErr := load(Root, id)
	if readErr != nil || j.Phase != os.Getenv("MEDIA_TOOL_HELPER_EXPECT") {
		t.Fatal("helper phase", readErr, j.Phase, j.Failure)
	}
	if j.Phase == "succeeded" && (e != nil || calls != 4) {
		t.Fatal("full entrypoint", e, calls)
	}
	if j.Phase == "failed" && e == nil {
		t.Fatal("failure hidden")
	}
	if j.Phase == "rolled_back" && (e != nil || j.Failure != "health_or_schema_failed") {
		t.Fatal("rollback must be complete and preserve original failure", e, j.Failure)
	}
	if j.Phase == "failed" && j.Failure != "backup_failed" {
		t.Fatal(j.Failure)
	}
	if j.Phase != "failed" && (j.Backup == "" || j.DumpSHA256 == "") {
		t.Fatal("missing verified pg_dump")
	}
}

// Minimal app fixture, in a separate container, not a mocked Health method.
func TestDockerReadyChild(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_READY_CHILD") != "1" {
		t.Skip("Docker app fixture only")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		flag := "/data/fail-health-once"
		if _, e := os.Stat(flag); e == nil {
			_ = os.Remove(flag)
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"status":"ready"}}`)
	})
	t.Fatal(http.ListenAndServe(":18080", mux))
}

func helperTestDB(t *testing.T) (*pgxpool.Pool, *url.URL) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" || os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("serialized media_tool_test only")
	}
	db, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	return db, u
}

func TestRealHelperEntrypointDocker(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_TEST_HELPER_DOCKER") != "1" {
		t.Skip("explicit isolated Docker fixture required")
	}
	db, dsn := helperTestDB(t)
	pg := os.Getenv("TEST_POSTGRES_CONTAINER")
	if pg == "" {
		pg = "media-tool-postgres-1"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	commands := func(t *testing.T) func(...string) []byte {
		return func(args ...string) []byte {
			t.Helper()
			c := exec.CommandContext(ctx, "docker", args...)
			b, e := c.CombinedOutput()
			if e != nil {
				t.Fatal("fixture Docker", args[0], e, string(b))
			}
			return b
		}
	}
	cli := commands(t)
	for _, image := range []string{helperFixtureOld, helperFixtureImage} {
		cli("pull", "--platform", "linux/amd64", image)
	}
	binaryDir := t.TempDir()
	// Build a CGO-free Linux fixture just as the release worker kernel tests do.
	// The parent Go suite remains -race; production helper budgets are unchanged.
	binary := filepath.Join(binaryDir, "helper.test")
	c := exec.CommandContext(ctx, "go", "test", "-c", "-o", binary, ".")
	c.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatal(e, string(b))
	}
	for _, tc := range []struct{ name, phase string }{{"success", "succeeded"}, {"backup_failure", "failed"}, {"health_failure", "rolled_back"}} {
		t.Run(tc.name, func(t *testing.T) {
			cli := commands(t)
			id, e := newID()
			if e != nil {
				t.Fatal(e)
			}
			project := "f8b-entry-" + id[:10]
			helperName := project + "-updater-" + id
			// Colima shares HOME, not /var/folders or external SSD volumes. Use
			// only this program's scoped fixture directory and remove it on exit.
			base, e := filepath.Abs("../../../data/private/f8b-helper-tests")
			if runtime.GOOS == "darwin" {
				home, err := os.UserHomeDir()
				if err != nil {
					t.Fatal(err)
				}
				base = filepath.Join(home, ".cache", "mediatool-f8b-helper-tests")
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = os.MkdirAll(base, 0700); e != nil {
				t.Fatal(e)
			}
			work, e := os.MkdirTemp(base, tc.name+"-")
			if e != nil {
				t.Fatal(e)
			}
			if e = os.Chmod(work, 0755); e != nil {
				t.Fatal(e)
			}
			data := filepath.Join(work, "data")
			root := filepath.Join(data, "online-update")
			if e = os.MkdirAll(root, 0777); e != nil {
				t.Fatal(e)
			}
			if e = os.Chmod(data, 0777); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(binary)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(work, "helper.test"), b, 0755); e != nil {
				t.Fatal(e)
			}
			fixtureDSN := *dsn
			fixtureDSN.Host = "testpg:5432"
			if tc.phase == "failed" {
				fixtureDSN.User = url.UserPassword(dsn.User.Username(), "intentionally-invalid-fixture-password")
			}
			q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
			before := fmt.Sprintf("name: %s\nservices:\n  app:\n    image: %s\n    platform: linux/amd64\n    user: '65532:65532'\n    entrypoint: [%s]\n    command: ['-test.run=^TestDockerReadyChild$']\n    environment:\n      MEDIA_TOOL_READY_CHILD: '1'\n      APP_ADDR: ':18080'\n      DATABASE_URL: %s\n    volumes:\n      - %s\n      - %s\n    read_only: true\n    cap_drop: [ALL]\n    security_opt: ['no-new-privileges:true']\n    healthcheck:\n      test: ['CMD', 'true']\n      interval: 1s\n      timeout: 1s\n      retries: 5\n", project, helperFixtureOld, q(filepath.Join(work, "helper.test")), q(fixtureDSN.String()), q(work+":"+work+":ro"), q(data+":/data:rw"))
			file := filepath.Join(work, "docker-compose.yml")
			if e = os.WriteFile(file, []byte(before), 0600); e != nil {
				t.Fatal(e)
			}
			compose := func(args ...string) []byte { return cli(append([]string{"compose", "-f", file}, args...)...) }
			restoreOwnership := func() {
				_ = exec.Command("docker", "run", "--rm", "--user", "0:0", "-v", work+":"+work+":rw", "--entrypoint", "/bin/chown", helperFixtureImage, "-R", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), work).Run()
			}
			t.Cleanup(func() {
				_ = exec.Command("docker", "rm", "-f", helperName).Run()
				_ = exec.Command("docker", "network", "disconnect", project+"_default", pg).Run()
				_ = exec.Command("docker", "compose", "-f", file, "down").Run()
				restoreOwnership()
				if e := os.RemoveAll(work); e != nil {
					t.Error("scoped fixture cleanup", e)
				}
				_, _ = db.Exec(context.Background(), `DELETE FROM audit_logs WHERE details->>'id'=$1`, id)
			})
			compose("up", "-d", "--pull", "never", "--wait")
			cli("network", "connect", "--alias", "testpg", project+"_default", pg)
			cid := strings.TrimSpace(string(compose("ps", "-q", "app")))
			endpoint := strings.TrimSpace(string(cli("context", "inspect", "--format", "{{.Endpoints.docker.Host}}")))
			socketPath := strings.TrimPrefix(endpoint, "unix://")
			inspectedDocker := newDocker()
			inspectedDocker.client.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			}}
			original, e := inspectedDocker.inspect(ctx, cid)
			if e != nil {
				t.Fatal(e)
			}
			j := Job{ID: id, Actor: "fixture-admin", FromVersion: "0.28.0", FromImage: helperFixtureOld, CreatedAt: time.Now().UTC(), Phase: "queued"}
			j.Target = updatecheck.Release{Metadata: updatecheck.Metadata{Version: "0.28.1", Commit: "1883c1850084932a116b7d38250824b02ac3f202", Digest: strings.TrimPrefix(helperFixtureImage, "dingding229/mediatool@"), MinimumSchema: 79, Schema: 79, Platform: "linux/amd64", Repository: updatecheck.Repository}, Tag: "v0.28.1"}
			p := newPlan(original, helperFixtureImage, id)
			raw, _ := json.Marshal(p)
			if strings.Contains(string(raw), "DATABASE_URL") || strings.Contains(string(raw), fixtureDSN.String()) {
				t.Fatal("plan leaked DSN")
			}
			if e = os.WriteFile(filepath.Join(root, id+".plan"), raw, 0600); e != nil {
				t.Fatal(e)
			}
			if e = save(root, j); e != nil {
				t.Fatal(e)
			}
			if e = audit(ctx, db, j.Actor, "online_update.requested", j); e != nil {
				t.Fatal(e)
			}
			if tc.phase == "rolled_back" {
				if e = os.WriteFile(filepath.Join(data, "fail-health-once"), []byte("fixture"), 0666); e != nil {
					t.Fatal(e)
				}
			}
			sock := strings.TrimPrefix(endpoint, "unix://")
			if runtime.GOOS == "darwin" {
				sock = "/var/run/docker.sock"
			}
			child := []string{"run", "--rm", "--name", helperName, "--label", label + "=" + id, "--label", "io.mediatool.project=" + project, "--user", "0:0", "--platform", "linux/amd64", "--network", project + "_default", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m", "-v", work + ":" + work + ":rw", "-v", data + ":/data:rw", "-v", sock + ":/var/run/docker.sock:rw", "-e", "MEDIA_TOOL_HELPER_CHILD=1", "-e", "MEDIA_TOOL_HELPER_ID=" + id, "-e", "MEDIA_TOOL_HELPER_EXPECT=" + tc.phase, "--entrypoint", filepath.Join(work, "helper.test"), helperFixtureImage, "-test.run=^TestDockerHelperChild$", "-test.v"}
			t.Log(string(cli(child...)))
			restoreOwnership()
			done, e := load(root, id)
			if e != nil || done.Phase != tc.phase {
				t.Fatal(e, done.Phase)
			}
			// Both helper and app reconciliation must be idempotent; backup failure has
			// deliberately invalid PG credentials, so ONLY the app's own pool can audit.
			s := &Service{DB: db, Root: root}
			for range 2 {
				if e = s.reconcileAudits(ctx); e != nil {
					t.Fatal(e)
				}
			}
			var count int
			var source string
			if e = db.QueryRow(ctx, `SELECT count(*),min(details->>'auditSource') FROM audit_logs WHERE action=$1 AND details->>'id'=$2`, "online_update."+tc.phase, id).Scan(&count, &source); e != nil || count != 1 {
				t.Fatal("terminal audit", e, count)
			}
			expectedSource := "helper"
			if tc.phase == "failed" {
				expectedSource = "reconciliation"
			}
			if source != expectedSource {
				t.Fatal("audit source", source)
			}
			var payload string
			if e = db.QueryRow(ctx, `SELECT details::text FROM audit_logs WHERE action=$1 AND details->>'id'=$2`, "online_update."+tc.phase, id).Scan(&payload); e != nil {
				t.Fatal(e)
			}
			pw, _ := fixtureDSN.User.Password()
			if strings.Contains(payload, "DATABASE_URL") || strings.Contains(payload, fixtureDSN.String()) || strings.Contains(payload, pw) {
				t.Fatal("audit leaked credentials")
			}
			live, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			expected := before
			if tc.phase == "succeeded" {
				expected = strings.Replace(before, helperFixtureOld, helperFixtureImage, 1)
			}
			if string(live) != expected {
				t.Fatal("unexpected Compose changes")
			}
			if tc.phase != "failed" {
				f, e := os.Open(filepath.Join(done.Backup, "database.dump"))
				if e != nil {
					t.Fatal(e)
				}
				restore := exec.CommandContext(ctx, "docker", "exec", "-i", pg, "pg_restore", "--list")
				restore.Stdin = f
				_, e = restore.Output()
				_ = f.Close()
				if e != nil {
					t.Fatal("dump unreadable", e)
				}
			}
			t.Logf("exported RunUpdater: phase=%s terminal audit=%s count=%d; real inspect/pg_dump/Compose/readyz/schema", tc.phase, source, count)
		})
	}
}
