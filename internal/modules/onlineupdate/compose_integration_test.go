package onlineupdate

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real Docker/Compose, but only a dedicated sleep/true fixture with no DB or
// business mounts. Opt in locally; unit tests also exercise a simulated daemon.
func TestRealComposeThroughAllowlist(t *testing.T) {
	image := os.Getenv("MEDIA_TOOL_TEST_UPDATE_IMAGE")
	if image == "" {
		t.Skip("explicit immutable fixture image required")
	}
	if !validImage(image) {
		t.Fatal("fixture must use owned Hub digest")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	id, e := newID()
	if e != nil {
		t.Fatal(e)
	}
	project := "f8b-policy-" + id[:10]
	root := t.TempDir()
	file := filepath.Join(root, "docker-compose.yml")
	source := "name: " + project + "\nservices:\n  app:\n    image: " + image + "\n    platform: linux/amd64\n    entrypoint: ['/bin/sh']\n    command: ['-c', 'sleep 3600']\n    user: '65532:65532'\n    read_only: true\n    cap_drop: [ALL]\n    security_opt: ['no-new-privileges:true']\n    healthcheck:\n      test: ['CMD', 'true']\n      interval: 1s\n      timeout: 1s\n      retries: 5\n"
	source = strings.Replace(source, "  app:\n", "  app:\n    depends_on: [postgres, redis]\n", 1)
	sibling := func(name string) string {
		return "  " + name + ":\n    image: " + image + "\n    platform: linux/amd64\n    entrypoint: ['/bin/sh']\n    command: ['-c', 'sleep 3600']\n    user: '65532:65532'\n    read_only: true\n    cap_drop: [ALL]\n"
	}
	core := source + sibling("postgres") + sibling("redis")
	source = core + sibling("emby") + sibling("clouddrive2")
	if e = os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	cli := func(args ...string) []byte {
		t.Helper()
		c := exec.CommandContext(ctx, "docker", append([]string{"compose", "-f", file}, args...)...)
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatal(e, string(b))
		}
		return b
	}
	cli("up", "-d", "--pull", "never", "--wait")
	defer func() {
		_ = os.WriteFile(file, []byte(source), 0600)
		c := exec.Command("docker", "compose", "-f", file, "down")
		if b, e := c.CombinedOutput(); e != nil {
			t.Error(e, string(b))
		}
	}()
	beforeSiblings := map[string]string{}
	for _, service := range []string{"postgres", "redis", "emby", "clouddrive2"} {
		beforeSiblings[service] = strings.TrimSpace(string(cli("ps", "-q", service)))
	}
	if e = os.WriteFile(file, []byte(core), 0600); e != nil {
		t.Fatal(e)
	}
	cid := strings.TrimSpace(string(cli("ps", "-q", "app")))
	b, e := exec.CommandContext(ctx, "docker", "inspect", cid).Output()
	if e != nil {
		t.Fatal(e)
	}
	var list []Container
	if e = json.Unmarshal(b, &list); e != nil {
		t.Fatal(e)
	}
	d := newDocker()
	endpoint, e := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if e != nil {
		t.Fatal(e)
	}
	sock := strings.TrimPrefix(strings.TrimSpace(string(endpoint)), "unix://")
	if !filepath.IsAbs(sock) {
		t.Fatal("local Unix Docker socket required")
	}
	d.client.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}
	d.self = list[0]
	d.target = image
	d.task = id
	// Public fixed repository, digest-only API pull, explicitly AMD64. No Docker
	// CLI credentials/config are passed to this client.
	oldTarget := d.target
	d.target = "dingding229/mediatool@sha256:e458207bb03f850eaf80f828fad1460c1e09f2456903af413b0fc8fb5fb27a3b"
	if _, e = d.request(ctx, "POST", "/images/create?fromImage="+urlQuery(d.target)+"&platform=linux%2Famd64", nil); e != nil {
		t.Fatal("immutable public pull API", e)
	}
	imageProof, e := d.request(ctx, "GET", "/images/"+d.target+"/json", nil)
	if e != nil {
		t.Fatal(e)
	}
	var proof struct{ Architecture string }
	if json.Unmarshal(imageProof, &proof) != nil || proof.Architecture != "amd64" {
		t.Fatal("AMD64 pull proof")
	}
	d.target = oldTarget
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log(r.Method, r.URL.String())
		d.proxy(w, r)
	}))
	defer srv.Close()
	c := exec.CommandContext(ctx, "docker", "compose", "-f", file, "up", "-d", "--no-deps", "--no-build", "--pull", "never", "--force-recreate", "--wait", "--wait-timeout", "30", "app")
	c.Env = append(os.Environ(), "DOCKER_HOST=tcp://"+strings.TrimPrefix(srv.URL, "http://"), "DOCKER_API_VERSION=1.47")
	out, e := c.CombinedOutput()
	if e != nil {
		t.Fatal("guarded Compose", e, string(out))
	}
	current := strings.TrimSpace(string(cli("ps", "-q", "app")))
	for service, id := range beforeSiblings {
		out, e := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Running}}", id).Output()
		if e != nil || strings.TrimSpace(string(out)) != id+" true" {
			t.Fatal("sibling changed", service, e, string(out))
		}
	}
	if current == cid {
		t.Fatal("app not recreated")
	}
	if _, e = d.request(ctx, "POST", "/containers/foreign/stop", nil); e == nil {
		t.Fatal("foreign stop permitted")
	}
}
