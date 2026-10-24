package onlineupdate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMountSemanticNormalization(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right string
		equal       bool
	}{
		{"short bind vs create_host_path", `{"Binds":["/media:/media:ro,rprivate"]}`, `{"Mounts":[{"Type":"bind","Source":"/media","Target":"/media","ReadOnly":true,"BindOptions":{"CreateMountpoint":true}}]}`, true},
		{"do not silently create source", `{"Binds":["/media:/media:ro"]}`, `{"Mounts":[{"Type":"bind","Source":"/media","Target":"/media","ReadOnly":true,"BindOptions":{"CreateMountpoint":false}}]}`, false},
		{"volume no copy", `{"Binds":["owned_data:/data:rw,nocopy"]}`, `{"Mounts":[{"Type":"volume","Source":"owned_data","Target":"/data","VolumeOptions":{"NoCopy":true}}]}`, true},
		{"propagation differs", `{"Binds":["/media:/media:rw,rshared"]}`, `{"Binds":["/media:/media:rw,rprivate"]}`, false},
		{"RW differs", `{"Binds":["/media:/media:rw"]}`, `{"Binds":["/media:/media:ro"]}`, false},
		{"unknown option", `{"Binds":["/media:/media:z"]}`, `{"Binds":["/media:/media:z"]}`, false},
		{"unknown typed option", `{"Mounts":[{"Type":"bind","Source":"/media","Target":"/media","BindOptions":{"Unknown":true}}]}`, `{}`, false},
		{"duplicate target", `{"Binds":["/a:/media:rw","/b:/media:rw"]}`, `{}`, false},
		{"recursive change", `{"Mounts":[{"Type":"bind","Source":"/media","Target":"/media","BindOptions":{"NonRecursive":true}}]}`, `{"Mounts":[{"Type":"bind","Source":"/media","Target":"/media"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a, b map[string]any
			_ = json.Unmarshal([]byte(tc.left), &a)
			_ = json.Unmarshal([]byte(tc.right), &b)
			x, e1 := mountSpecs(a)
			y, e2 := mountSpecs(b)
			equal := e1 == nil && e2 == nil && reflect.DeepEqual(x, y)
			if equal != tc.equal {
				t.Fatal(equal, e1, e2, x, y)
			}
		})
	}
}
func TestProductionShapeDryRunNeverWritesDocker(t *testing.T) {
	d := testDocker()
	d.self.Config["Hostname"] = d.self.ID[:12]
	binds := []any{}
	for i := 0; i < 9; i++ {
		p := "/mount" + strings.Repeat("x", i)
		binds = append(binds, p+":"+p+":ro")
	}
	d.self.HostConfig["Binds"] = binds
	d.self.NetworkSettings.Networks = map[string]json.RawMessage{"owned_default": json.RawMessage(`{"Aliases":["app","owned-app-1"],"IPAMConfig":null}`)}
	writes, reads := 0, 0
	d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			writes++
			t.Error("dry run wrote Docker")
		}
		reads++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Id":"sha256:fixture","Config":{}}`)), Header: make(http.Header)}, nil
	})}
	body, e := d.appCreate(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = d.preflightCreate(body); e != nil {
		t.Fatal(e, string(body))
	}
	var c map[string]any
	_ = json.Unmarshal(body, &c)
	h := c["HostConfig"].(map[string]any)
	// Simulate Compose 5 Binds -> Compose 2 typed Mounts, with same semantics.
	h["Binds"] = binds[:8]
	h["Mounts"] = []any{map[string]any{"Type": "bind", "Source": "/mountxxxxxxxx", "Target": "/mountxxxxxxxx", "ReadOnly": true, "BindOptions": map[string]any{"CreateMountpoint": true}}}
	body, _ = json.Marshal(c)
	if e = d.preflightCreate(body); e != nil {
		t.Fatal("equivalent mixed mounts refused", e)
	}
	h["Mounts"].([]any)[0].(map[string]any)["BindOptions"].(map[string]any)["CreateMountpoint"] = false
	body, _ = json.Marshal(c)
	if e = d.preflightCreate(body); e == nil {
		t.Fatal("create_host_path semantics widened")
	}
	if writes != 0 || reads != 1 {
		t.Fatal(reads, writes)
	}
}
func TestStderrEvidenceNeverPersistsEnvironmentSecrets(t *testing.T) {
	d := testDocker()
	d.self.Config["Env"] = []any{"DATABASE_URL=postgres://owner:secret-pass@db/media_tool_test", "KEY=opaque-private-key"}
	r := &runner{d: d, root: t.TempDir()}
	if e := r.evidence("pg_dump", []byte(`postgres://owner:secret-pass@db/media_tool_test password=secret-pass opaque-private-key https://115.com/secret?api_key=123`), true); e != nil {
		t.Fatal(e)
	}
	// Evidence is intentionally not returned by any administrator API.
	files, e := filepath.Glob(filepath.Join(r.root, d.task+".evidence", "*-pg_dump.stderr.json"))
	if e != nil || len(files) != 1 {
		t.Fatal(e, files)
	}
	b, e := os.ReadFile(files[0])
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"secret-pass", "opaque-private-key", "postgres://", "https://", "api_key=123"} {
		if strings.Contains(string(b), v) {
			t.Fatal("credential persisted", v)
		}
	}
}
