package onlineupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stringRepeat(s string, n int) string { return strings.Repeat(s, n) }
func testDocker() *docker {
	d := newDocker()
	d.self = Container{ID: strings.Repeat("a", 64), Name: "/owned-app-1", Image: "sha256:" + strings.Repeat("b", 64), Config: map[string]any{"Image": "dingding229/mediatool@sha256:" + strings.Repeat("b", 64), "User": "65532:65532", "Labels": map[string]any{"com.docker.compose.project": "owned", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": "/owned"}}, HostConfig: map[string]any{"ReadonlyRootfs": true}}
	d.target = "dingding229/mediatool@sha256:" + strings.Repeat("c", 64)
	d.task = strings.Repeat("d", 32)
	d.self.Mounts = []Mount{{Type: "bind", Source: "/owned/data", Destination: "/data", RW: true}}
	return d
}
func TestDockerPolicyDeniesNonProjectAndPowerfulAPIs(t *testing.T) {
	d := testDocker()
	for _, p := range []string{"/containers/foreign/stop", "/containers/foreign/start", "/containers/foreign", "/containers/foreign/exec", "/exec/a/start", "/images/prune", "/volumes/prune", "/volumes/business", "/networks/foreign", "/networks/create", "/build", "/plugins/create", "/swarm/init", "/containers/../images/prune", "/containers/%2e%2e/stop"} {
		for _, m := range []string{"GET", "POST", "DELETE"} {
			if d.allowed(m, p, nil) {
				t.Fatal(m, p)
			}
		}
	}
	for _, p := range []string{"/images/create?fromImage=evil/repo@sha256:" + strings.Repeat("a", 64), "/images/create?fromImage=dingding229/mediatool:latest", "/images/create?fromImage=" + urlQuery(d.target) + "&tag=latest"} {
		if d.allowed("POST", p, nil) {
			t.Fatal(p)
		}
	}
	if !policyCheck(d) {
		t.Fatal("self-check")
	}
}
func TestDockerOnlyPullsReleaseDigestAndFailsWithoutSideEffects(t *testing.T) {
	d := testDocker()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != apiVersion+"/images/create" || r.URL.Query().Get("fromImage") != d.target || r.Header.Get("Authorization") != "" {
			t.Error(r.URL)
		}
		_, _ = w.Write([]byte(`{"error":"pull denied"}`))
	}))
	defer srv.Close()
	d.base = srv.URL
	d.client = srv.Client()
	if _, e := d.request(context.Background(), "POST", "/images/create?fromImage="+urlQuery(d.target)+"&platform=linux%2Famd64", nil); e == nil {
		t.Fatal("pull stream failure ignored")
	}
	if _, e := d.request(context.Background(), "POST", "/volumes/prune", nil); e == nil {
		t.Fatal("prune allowed")
	}
	if calls != 1 {
		t.Fatal("denied API reached daemon", calls)
	}
}
func TestTemporaryUpdaterConfigIsExactAndAutoRemoved(t *testing.T) {
	d := testDocker()
	b, e := json.Marshal(d.helperConfig())
	if e != nil || !d.allowed("POST", "/containers/create?name="+d.helperName(), b) {
		t.Fatal(e, string(b))
	}
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	body["User"] = "65532:65532"
	b, _ = json.Marshal(body)
	if d.allowed("POST", "/containers/create?name="+d.helperName(), b) {
		t.Fatal("modified helper accepted")
	}
}
func TestAppCreateNeverExpandsPrivileges(t *testing.T) {
	d := testDocker()
	c := map[string]any{"Image": d.target, "User": "65532:65532", "Labels": map[string]any{"com.docker.compose.project": "owned", "com.docker.compose.service": "app"}, "HostConfig": map[string]any{"ReadonlyRootfs": true}}
	b, _ := json.Marshal(c)
	if !d.validCreate(d.appName(), b) {
		t.Fatal("unchanged app refused", string(b))
	}
	for _, mutate := range []func(map[string]any){func(c map[string]any) { c["Image"] = "evil:latest" }, func(c map[string]any) { c["User"] = "0:0" }, func(c map[string]any) { c["Env"] = []string{"SECRET=unexpected"} }, func(c map[string]any) { c["HostConfig"].(map[string]any)["Privileged"] = true }, func(c map[string]any) { c["HostConfig"].(map[string]any)["Binds"] = []string{"/:/host:rw"} }} {
		var v map[string]any
		_ = json.Unmarshal(b, &v)
		mutate(v)
		raw, _ := json.Marshal(v)
		if d.validCreate(d.appName(), raw) {
			t.Fatal("privilege expansion accepted", string(raw))
		}
	}
}

func TestRollbackTemporaryNameOnlyUsesKnownOwnedAppIDs(t *testing.T) {
	d := testDocker()
	current := strings.Repeat("e", 64)
	foreign := strings.Repeat("f", 64)
	for _, prefix := range []string{current[:12], current} {
		if d.validAppName(prefix + "_" + d.appName()) {
			t.Fatal("unverified app prefix accepted")
		}
	}
	d.ids[current] = true
	for _, prefix := range []string{current[:12], current} {
		if !d.validAppName(prefix+"_"+d.appName()) || !d.scoped(prefix+"_"+d.appName()) {
			t.Fatal("owned rollback name refused")
		}
	}
	for _, name := range []string{foreign[:12] + "_" + d.appName(), current[:12] + "_foreign", current + "_foreign"} {
		if d.validAppName(name) || d.scoped(name) {
			t.Fatal("foreign name accepted")
		}
	}
}
