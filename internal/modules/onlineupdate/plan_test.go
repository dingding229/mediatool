package onlineupdate

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedPlanNeverSerializesParentSecrets(t *testing.T) {
	d := testDocker()
	d.self.Config["Env"] = []string{"APP_SECRET_KEY=master-secret-fixture", "DATABASE_URL=postgres://private-credential"}
	d.self.Config["Cmd"] = []string{"private-command"}
	d.self.Config["Labels"].(map[string]any)["operator.password"] = "private-label"
	d.self.HostConfig["LogConfig"] = map[string]any{"password": "private-log"}
	p := newPlan(d.self, d.target, d.task)
	b, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"master-secret-fixture", "private-credential", "private-command", "private-label", "private-log", "APP_SECRET_KEY", "DATABASE_URL"} {
		if strings.Contains(string(b), s) {
			t.Fatal("secret entered shared plan", s)
		}
	}
	if len(p.ConfigurationProof) != 64 || p.Self.Config["Env"] != nil || p.Self.HostConfig != nil || d.self.Config["Env"] == nil {
		t.Fatal("plan projection/proof")
	}
}
func TestOriginalConfigurationRestoresFromPrivateSnapshotAndRejectsMismatch(t *testing.T) {
	d := testDocker()
	work := t.TempDir()
	d.self.Config["Labels"].(map[string]any)["com.docker.compose.project.working_dir"] = work
	d.self.Config["Env"] = []string{"APP_SECRET_KEY=private-fixture"}
	original := d.self
	p := newPlan(original, d.target, d.task)
	d.self = p.Self
	raw, e := json.Marshal(original)
	if e != nil {
		t.Fatal(e)
	}
	d.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	j := Job{Phase: "queued", FromImage: fmtString(original.Config["Image"])}
	c, e := originalContainer(t.Context(), d, p, j)
	if e != nil || env(c, "APP_SECRET_KEY") != "private-fixture" {
		t.Fatal(e)
	}
	j.Phase = "checking"
	j.Backup = filepath.Join(work, "private", "backup")
	if e = os.MkdirAll(j.Backup, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(j.Backup, "container.before.json")
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	c, e = originalContainer(t.Context(), d, p, j)
	if e != nil || env(c, "APP_SECRET_KEY") != "private-fixture" {
		t.Fatal(e)
	}
	original.Config["Env"] = []string{"APP_SECRET_KEY=changed-private-fixture"}
	raw, _ = json.Marshal(original)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = originalContainer(t.Context(), d, p, j); e == nil {
		t.Fatal("changed original configuration accepted")
	}
}
func TestConfigurationProofCanonicalMountAndEnvironmentOrder(t *testing.T) {
	d := testDocker()
	c := d.self
	c.Mounts = []Mount{{Type: "bind", Source: "/b", Destination: "/b"}, {Type: "bind", Source: "/a", Destination: "/a"}}
	c.Config["Env"] = []string{"B=2", "A=1"}
	before := configurationProof(c)
	c.Mounts[0], c.Mounts[1] = c.Mounts[1], c.Mounts[0]
	c.Config["Env"] = []string{"A=1", "B=2"}
	if configurationProof(c) != before {
		t.Fatal("ordering changed proof")
	}
	c.Mounts[0].RW = true
	if configurationProof(c) == before {
		t.Fatal("mount permission change hidden")
	}
}
