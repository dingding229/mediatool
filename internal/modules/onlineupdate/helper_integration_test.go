package onlineupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"

	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The mounted fixture binary uses today's schema. Synthetic migration uses
// Schema+1; never delete a real migration ledger row when the release advances.
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
	// Probe from the SAME VM clock as the helper journal. Host/Colima clock
	// offsets cannot be treated as service downtime. DNS app follows the old/new
	// scoped service; do not reuse keepalive connections across the switch.
	type sample struct {
		start, end time.Time
		ok         bool
	}
	samples := []sample{}
	var mu sync.Mutex
	probeCtx, stopProbe := context.WithCancel(t.Context())
	defer stopProbe()
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		client := http.Client{Timeout: 300 * time.Millisecond, Transport: &http.Transport{DisableKeepAlives: true}}
		for {
			req, _ := http.NewRequestWithContext(probeCtx, "GET", "http://app:18080/readyz", nil)
			start := time.Now()
			resp, err := client.Do(req)
			ok := err == nil && resp.StatusCode == 200
			if resp != nil {
				_ = resp.Body.Close()
			}
			if probeCtx.Err() != nil {
				return
			}
			mu.Lock()
			samples = append(samples, sample{start, time.Now(), ok})
			mu.Unlock()
			select {
			case <-probeCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	time.Sleep(time.Second)
	if os.Getenv("MEDIA_TOOL_HELPER_CASE") == "start_failure" {
		d := newDocker()
		upstream := d.client.Transport
		d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/start") && !strings.Contains(req.URL.Path, p.Self.ID) {
				db, _ := helperTestDB(t)
				if _, err := db.Exec(t.Context(), `UPDATE f8b_flow_probe SET value='after-backup'`); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"injected new-container start failure"}`))}, nil
			}
			return upstream.RoundTrip(req)
		})
		e = runUpdater(t.Context(), Root, id, d)
	} else if strings.HasPrefix(os.Getenv("MEDIA_TOOL_HELPER_CASE"), "image_config_") {
		d := newDocker()
		upstream := d.client.Transport
		derived := os.Getenv("MEDIA_TOOL_GUARD_IMAGE_ID")
		if !strings.HasPrefix(derived, "sha256:") || len(derived) != 71 {
			t.Fatal("real derived image ID required")
		}
		d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" && req.URL.Path == apiVersion+"/images/"+p.TargetImage+"/json" {
				clone := req.Clone(req.Context())
				u := *req.URL
				u.Path = apiVersion + "/images/" + derived + "/json"
				clone.URL = &u
				return upstream.RoundTrip(clone)
			}
			return upstream.RoundTrip(req)
		})
		e = runUpdater(t.Context(), Root, id, d)
	} else {
		if os.Getenv("MEDIA_TOOL_HELPER_CASE") == "backup_failure" {
			_ = os.Setenv("PATH", labels(p.Self)["com.docker.compose.project.working_dir"]+"/bin:"+os.Getenv("PATH"))
		}
		e = RunUpdater(t.Context(), id)
	}
	time.Sleep(time.Second)
	stopProbe()
	<-probeDone
	j, readErr := load(Root, id)
	var windowStart, windowEnd time.Time
	for _, event := range j.Events {
		if event.Phase == "applying" {
			windowStart = event.At
		}
		if event.Phase == j.Phase {
			windowEnd = event.At
		}
	}
	stable, failures := 0, 0
	mu.Lock()
	for _, v := range samples {
		if !windowStart.IsZero() && !v.end.Before(windowStart) && !v.start.After(windowEnd) {
			continue
		}
		stable++
		if !v.ok {
			failures++
			t.Logf("unexpected readyz interval %s–%s; switch %s–%s", v.start, v.end, windowStart, windowEnd)
		}
	}
	mu.Unlock()
	if stable < 10 || failures != 0 {
		t.Fatal("readyz outside expected switch window", stable, failures)
	}
	t.Logf("continuous readyz: %d stable probes, %d failures outside expected switch window (same-clock measurement)", stable, failures)

	if readErr != nil || j.Phase != os.Getenv("MEDIA_TOOL_HELPER_EXPECT") {
		t.Fatal("helper phase", readErr, j.Phase, j.Failure, e)
	}
	if j.Phase == "succeeded" && (e != nil || calls != 6) {
		t.Fatal("full entrypoint", e, calls)
	}
	if j.Phase == "failed" && e == nil {
		t.Fatal("failure hidden")
	}
	if j.Phase == "rolled_back" && (e != nil || (j.Failure != "health_or_schema_failed" && j.Failure != "apply_failed")) {
		t.Fatal("rollback must be complete and preserve original failure", e, j.Failure)
	}
	if j.Phase == "failed" && (j.Failure != "backup_failed" && j.Failure != "preflight_rejected" && j.Failure != "image_startup_config_changed") {
		t.Fatal(j.Failure)
	}
	if j.Phase != "failed" && (j.Backup == "" || j.DumpSHA256 == "") {
		t.Fatal("missing verified pg_dump")
	}
}

// Actual app UID, before any host fixture ownership cleanup. Admission creates
// the execution lock as app; reconciliation must never depend on root access.
func TestDockerAppAuditChild(t *testing.T) {
	mode := os.Getenv("MEDIA_TOOL_APP_AUDIT_CHILD")
	if mode == "" {
		t.Skip("Docker app child only")
	}
	if os.Getuid() != 65532 {
		t.Fatal("real non-root app UID required")
	}
	db, _ := helperTestDB(t)
	checker := updatecheck.New(db, nil)
	checker.Client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture unavailable"))}, nil
	})
	service := New(db, checker)
	if mode == "admission" {
		id := os.Getenv("MEDIA_TOOL_HELPER_ID")
		if e := service.reconcileAudits(t.Context()); e != nil {
			t.Fatal(e)
		}
		if _, e := service.Start(t.Context(), "fixture-preflight-"+id, "0.28.1", strings.TrimPrefix(helperFixtureImage, "dingding229/mediatool@")); e == nil {
			t.Fatal("unavailable release admitted")
		}
	} else if mode == "seed" {
		dir := os.Getenv("MEDIA_TOOL_SEED_DIR")
		id := os.Getenv("MEDIA_TOOL_HELPER_ID")
		raw, e := os.ReadFile(filepath.Join(dir, "fixture-plan.json"))
		if e != nil || bytes.Contains(raw, []byte("DATABASE_URL")) {
			t.Fatal("redacted fixture plan", e)
		}
		if e = atomicFile(filepath.Join(Root, id+".plan"), raw); e != nil {
			t.Fatal(e)
		}
		raw, e = os.ReadFile(filepath.Join(dir, "fixture-job.json"))
		if e != nil {
			t.Fatal(e)
		}
		var job Job
		if e = json.Unmarshal(raw, &job); e != nil || job.ID != id {
			t.Fatal(e)
		}
		if e = save(Root, job); e != nil {
			t.Fatal(e)
		}
	} else {
		id := os.Getenv("MEDIA_TOOL_HELPER_ID")
		st, e := os.Stat(filepath.Join(Root, id+".plan"))
		if e != nil || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Uid != 65532 {
			t.Fatal("plan must be written by the real app UID before helper, not repaired by cleanup", e)
		}
		for range 2 {
			if e := service.reconcileAudits(t.Context()); e != nil {
				t.Fatal("real app UID cannot reconcile", e)
			}
		}
	}
	st, e := os.Stat(filepath.Join(Root, "execution.lock"))
	if e != nil || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Uid != 65532 {
		t.Fatal("app must own execution lock", e)
	}
}

// Minimal app fixture, in a separate container, not a mocked Health method.
func TestDockerReadyChild(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_READY_CHILD") != "1" {
		t.Skip("Docker app fixture only")
	}

	host, _ := os.Hostname()
	oldHost, _ := os.ReadFile("/data/original-host")
	newApp := string(oldHost) != "" && string(oldHost) != host
	mode, _ := os.ReadFile("/data/flow-case")
	if newApp {
		db, e := pgxpool.New(t.Context(), os.Getenv("DATABASE_URL"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(t.Context(), `UPDATE f8b_flow_probe SET value='after-backup'`); e != nil {
			t.Fatal(e)
		}
		if string(mode) == "migration_failure" {
			if _, e = db.Exec(t.Context(), fmt.Sprintf(`INSERT INTO goose_db_version(version_id,is_applied)VALUES(%d,true);CREATE TABLE f8b_flow_added(id int)`, buildinfo.Schema+1)); e != nil {
				t.Fatal(e)
			}
		}
		db.Close()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if newApp && (string(mode) == "health_failure" || string(mode) == "migration_failure") {
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
	cases := []struct{ name, phase string }{{"preflight_rejected", "failed"}, {"start_failure", "rolled_back"}, {"migration_failure", "rolled_back"}, {"health_failure", "rolled_back"}, {"success", "succeeded"}, {"backup_failure", "failed"}}
	for _, c := range startupImageCases {
		cases = append(cases, struct{ name, phase string }{"image_config_" + c.field, "failed"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := commands(t)
			id, e := newID()
			if e != nil {
				t.Fatal(e)
			}
			project := "f8b-entry-" + id[:10]
			helperName := project + "-updater-" + id
			derived := ""
			if strings.HasPrefix(tc.name, "image_config_") {
				field := strings.TrimPrefix(tc.name, "image_config_")
				instruction := ""
				for _, c := range startupImageCases {
					if c.field == field {
						instruction = c.instruction
					}
				}
				if instruction == "" {
					t.Fatal("missing Dockerfile mutation", field)
				}
				tag := "f8b-startup-config-" + id + ":fixture"
				build := exec.CommandContext(ctx, "docker", "build", "--platform", "linux/amd64", "--pull=false", "--network=none", "-t", tag, "-")
				build.Stdin = strings.NewReader("FROM " + helperFixtureImage + "\n" + instruction + "\n")
				if out, err := build.CombinedOutput(); err != nil {
					t.Fatal("isolated config-only image", err, string(out))
				}
				t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", tag).Run() })
				derived = strings.TrimSpace(string(cli("image", "inspect", "--format", "{{.Id}}", tag)))
				inspectConfig := func(ref string) map[string]any {
					cmd := exec.CommandContext(ctx, "docker", "image", "inspect", ref)
					cmd.Env = append(os.Environ(), "DOCKER_API_VERSION=1.47")
					raw, err := cmd.Output()
					if err != nil {
						t.Fatal(err)
					}
					var images []struct{ Config map[string]any }
					if json.Unmarshal(raw, &images) != nil || len(images) != 1 {
						t.Fatal("invalid fixture image config")
					}
					return images[0].Config
				}
				baseConfig, changedConfig := inspectConfig(helperFixtureImage), inspectConfig(tag)
				keys := map[string]bool{}
				for k := range baseConfig {
					keys[k] = true
				}
				for k := range changedConfig {
					keys[k] = true
				}
				changed := []string{}
				for k := range keys {
					if k == "Labels" {
						continue
					}
					a, aok := baseConfig[k]
					b, bok := changedConfig[k]
					if aok != bok || !reflect.DeepEqual(a, b) {
						changed = append(changed, k)
					}
				}
				// Docker's CMD builder also materializes ArgsEscaped=false. Keep
				// that difference visible and checked; do not normalize it away.
				sort.Strings(changed)
				expected := []string{field}
				if field == "Cmd" {
					expected = []string{"ArgsEscaped", "Cmd"}
				}
				if !reflect.DeepEqual(changed, expected) {
					t.Fatal("unexpected fixture startup changes", field, changed)
				}
			}

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
			appDSN := *dsn
			appDSN.Host = "testpg:5432"
			fixtureDSN := appDSN

			q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
			before := fmt.Sprintf("name: %s\nservices:\n  app:\n    image: %s\n    platform: linux/amd64\n    user: '65532:65532'\n    entrypoint: [%s]\n    command: ['-test.run=^TestDockerReadyChild$']\n    environment:\n      MEDIA_TOOL_READY_CHILD: '1'\n      APP_ADDR: ':18080'\n      DATABASE_URL: %s\n    volumes:\n      - %s\n      - %s\n    read_only: true\n    cap_drop: [ALL]\n    security_opt: ['no-new-privileges:true']\n    healthcheck:\n      test: ['CMD', 'true']\n      interval: 1s\n      timeout: 1s\n      retries: 5\n", project, helperFixtureOld, q(filepath.Join(work, "helper.test")), q(fixtureDSN.String()), q(work+":"+work+":ro"), q(data+":/data:rw"))
			// Real production-shaped mix of short/long mounts, not just two binds.
			extra := ""
			for i := 0; i < 8; i++ {
				source := filepath.Join(work, fmt.Sprintf("fixture%d", i))
				if e = os.Mkdir(source, 0755); e != nil {
					t.Fatal(e)
				}
				suffix := ":ro"
				if tc.name == "preflight_rejected" && i == 0 {
					suffix = ":ro,z"
				}
				if i%2 == 0 {
					extra += "      - " + q(source+":/fixture"+fmt.Sprint(i)+suffix) + "\n"
				} else {
					extra += fmt.Sprintf("      - type: bind\n        source: %s\n        target: /fixture%d\n        read_only: true\n        bind:\n          create_host_path: %t\n", q(source), i, i%3 != 0)
				}
			}
			portListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			fixedPort := portListener.Addr().(*net.TCPAddr).Port
			_ = portListener.Close()
			before = strings.Replace(before, "    read_only: true\n", extra+fmt.Sprintf("    ports: ['127.0.0.1:%d:18080']\n    read_only: true\n", fixedPort), 1)
			before = strings.Replace(before, "test: ['CMD', 'true']", "test: ['CMD-SHELL', 'test ! -f /data/unhealthy || test \"$(hostname)\" = \"$(cat /data/original-host)\"']", 1)
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
				_ = exec.Command("docker", "compose", "-f", file, "down", "--remove-orphans").Run()
				restoreOwnership()
				if e := os.RemoveAll(work); e != nil {
					t.Error("scoped fixture cleanup", e)
				}
				_, _ = db.Exec(context.Background(), `DELETE FROM audit_logs WHERE details->>'id'=$1 OR actor=$2`, id, "fixture-preflight-"+id)
			})
			compose("up", "-d", "--pull", "never", "--wait")
			cli("network", "connect", "--alias", "testpg", project+"_default", pg)
			cid := strings.TrimSpace(string(compose("ps", "-q", "app")))
			originalStarted := strings.TrimSpace(string(cli("inspect", "--format", "{{.State.StartedAt}}", cid)))
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
			j.Target = updatecheck.Release{Metadata: updatecheck.Metadata{Version: "0.28.1", Commit: "1883c1850084932a116b7d38250824b02ac3f202", Digest: strings.TrimPrefix(helperFixtureImage, "dingding229/mediatool@"), MinimumSchema: buildinfo.MinimumSchema, Schema: buildinfo.Schema, Platform: "linux/amd64", Repository: updatecheck.Repository}, Tag: "v0.28.1"}
			sock := strings.TrimPrefix(endpoint, "unix://")
			if runtime.GOOS == "darwin" {
				sock = "/var/run/docker.sock"
			}
			socketGroup := strings.TrimSpace(string(cli("run", "--rm", "--platform", "linux/amd64", "-v", sock+":/var/run/docker.sock:rw", "--entrypoint", "/bin/stat", helperFixtureImage, "-c", "%g", "/var/run/docker.sock")))
			if tc.name == "migration_failure" {
				j.Target.Schema = buildinfo.Schema + 1
			}
			appChild := func(mode string) {
				t.Log(string(cli("run", "--rm", "--platform", "linux/amd64", "--user", "65532:65532", "--group-add", socketGroup, "-v", sock+":/var/run/docker.sock:rw", "--read-only", "--cap-drop", "ALL", "--network", project+"_default", "-v", work+":"+work+":ro", "-v", data+":/data:rw", "-e", "MEDIA_TOOL_APP_AUDIT_CHILD="+mode, "-e", "MEDIA_TOOL_SEED_DIR="+work, "-e", "MEDIA_TOOL_HELPER_ID="+id, "-e", "TEST_DATABASE_URL="+appDSN.String(), "-e", "MEDIA_TOOL_TEST_LOCKED=1", "--entrypoint", filepath.Join(work, "helper.test"), helperFixtureImage, "-test.run=^TestDockerAppAuditChild$", "-test.v")))
			}
			rootOwner := func(owner string) {
				cli("run", "--rm", "--user", "0:0", "-v", data+":/data:rw", "--entrypoint", "/bin/chown", helperFixtureImage, owner, "/data/online-update")
			}
			if e = os.Chmod(root, 0700); e != nil {
				t.Fatal(e)
			}
			rootOwner("65532:65532")
			appChild("admission") // The production Start path prepares app-owned locks.
			if _, e = db.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS f8b_flow_probe,f8b_flow_added;DELETE FROM goose_db_version WHERE version_id=%d;CREATE TABLE f8b_flow_probe(value text);INSERT INTO f8b_flow_probe VALUES('before')`, buildinfo.Schema+1)); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(data, "original-host"), []byte(fmtString(original.Config["Hostname"])), 0644); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(data, "flow-case"), []byte(tc.name), 0644); e != nil {
				t.Fatal(e)
			}
			if tc.name == "health_failure" || tc.name == "migration_failure" {
				if e = os.WriteFile(filepath.Join(data, "unhealthy"), nil, 0644); e != nil {
					t.Fatal(e)
				}
			}
			if tc.name == "backup_failure" {
				if e = os.Mkdir(filepath.Join(work, "bin"), 0755); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(work, "bin", "pg_dump"), []byte("#!/bin/sh\necho 'injected dump failure' >&2\nexit 43\n"), 0755); e != nil {
					t.Fatal(e)
				}
			}
			p := newPlan(original, helperFixtureImage, id)
			raw, _ := json.Marshal(p)
			if strings.Contains(string(raw), "DATABASE_URL") || strings.Contains(string(raw), fixtureDSN.String()) {
				t.Fatal("plan leaked DSN")
			}
			// These inputs contain no credentials. The real non-root app process
			// writes the 0600 shared plan/job, exactly as production admission does.
			if e = os.WriteFile(filepath.Join(work, "fixture-plan.json"), raw, 0644); e != nil {
				t.Fatal(e)
			}
			raw, e = json.Marshal(j)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(work, "fixture-job.json"), raw, 0644); e != nil {
				t.Fatal(e)
			}
			appChild("seed")
			if e = audit(ctx, db, j.Actor, "online_update.requested", j); e != nil {
				t.Fatal(e)
			}
			child := []string{"run", "--name", helperName, "--label", label + "=" + id, "--label", "io.mediatool.project=" + project, "--user", "0:0", "--platform", "linux/amd64", "--network", project + "_default", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m", "-v", work + ":" + work + ":rw", "-v", data + ":/data:rw", "-v", sock + ":/var/run/docker.sock:rw", "-e", "MEDIA_TOOL_HELPER_CHILD=1", "-e", "MEDIA_TOOL_HELPER_ID=" + id, "-e", "MEDIA_TOOL_HELPER_EXPECT=" + tc.phase, "-e", "MEDIA_TOOL_HELPER_CASE=" + tc.name, "-e", "MEDIA_TOOL_GUARD_IMAGE_ID=" + derived, "-e", "TEST_DATABASE_URL=" + appDSN.String(), "-e", "MEDIA_TOOL_TEST_LOCKED=1", "--entrypoint", filepath.Join(work, "helper.test"), helperFixtureImage, "-test.run=^TestDockerHelperChild$", "-test.v"}
			t.Log(string(cli(child...)))
			appChild("reconcile") // Must pass BEFORE cleanup can chown any fixture files.
			if e := exec.CommandContext(ctx, "docker", "inspect", helperName).Run(); e == nil {
				t.Fatal("stopped helper was not cleaned after durable audit")
			}

			restoreOwnership()
			done, e := load(root, id)
			if e != nil || done.Phase != tc.phase {
				t.Fatal(e, done.Phase)
			}
			if strings.HasPrefix(tc.name, "image_config_") {
				if done.Phase != "failed" || done.Failure != "image_startup_config_changed" || done.Backup != "" || done.DumpSHA256 != "" {
					t.Fatal("startup change must fail before backup", done.Phase, done.Failure)
				}
				for _, event := range done.Events {
					if event.Phase == "backing_up" || event.Phase == "applying" || event.Phase == "rolling_back" {
						t.Fatal("startup guard ran too late", event.Phase)
					}
				}
				// No backup directory exists, and current app has never been stopped.
				if entries, err := os.ReadDir(filepath.Join(work, "private")); err == nil && len(entries) > 0 {
					t.Fatal("guard created backup")
				}
				now := strings.TrimSpace(string(cli("inspect", "--format", "{{.State.StartedAt}}", cid)))
				if now != originalStarted {
					t.Fatal("old app was restarted before guard rejection")
				}
			}
			// Both helper and app reconciliation must be idempotent, including backup failure.
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
			var marker string
			if e = db.QueryRow(ctx, `SELECT value FROM f8b_flow_probe`).Scan(&marker); e != nil {
				t.Fatal(e)
			}
			expectedMarker := "before"
			if tc.name == "success" || tc.name == "health_failure" || tc.name == "start_failure" {
				expectedMarker = "after-backup"
			}
			if marker != expectedMarker {
				t.Fatal("schema-conditioned DB rollback", tc.name, marker)
			}
			var actualSchema int64
			var absent bool
			if e = db.QueryRow(ctx, `SELECT max(version_id) FILTER(WHERE is_applied), to_regclass('public.f8b_flow_added') IS NULL FROM goose_db_version`).Scan(&actualSchema, &absent); e != nil || actualSchema != buildinfo.Schema || !absent {
				t.Fatal("actual migration rollback", e, actualSchema, absent)
			}
			current := strings.TrimSpace(string(compose("ps", "-q", "app")))
			if tc.phase != "succeeded" && current != cid {
				t.Fatal("rollback did not retain original container ID", current, cid)
			}
			if tc.phase == "succeeded" {
				if e := exec.CommandContext(ctx, "docker", "inspect", cid).Run(); e == nil {
					t.Fatal("old container not removed after success")
				}
			}
			if _, e = db.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS f8b_flow_probe,f8b_flow_added;DELETE FROM goose_db_version WHERE version_id=%d`, buildinfo.Schema+1)); e != nil {
				t.Fatal(e)
			}
			t.Logf("exported RunUpdater: phase=%s terminal audit=%s count=%d; real inspect/pg_dump/Compose/readyz/schema", tc.phase, source, count)
		})
	}
}
