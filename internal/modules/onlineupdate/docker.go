package onlineupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	deferutil "media-tool/internal/platform/cleanup"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type dockerError struct{ code int }

func (e dockerError) Error() string { return fmt.Sprintf("Docker operation HTTP %d", e.code) }

const socket = "/var/run/docker.sock"
const apiVersion = "/v1.47"

var containerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

type Mount struct {
	Type, Source, Destination, Name string
	RW                              bool
}
type Container struct {
	ID              string `json:"Id"`
	Name, Image     string
	Config          map[string]any
	HostConfig      map[string]any
	Mounts          []Mount
	NetworkSettings struct{ Networks map[string]json.RawMessage }
	State           struct {
		Running bool
		Health  struct{ Status string }
	}
}

func stringsValue(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := []string{}
		for _, s := range x {
			if v, ok := s.(string); ok {
				out = append(out, v)
			}
		}
		return out
	}
	return nil
}
func labels(c Container) map[string]string {
	out := map[string]string{}
	if m, ok := c.Config["Labels"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
func env(c Container, k string) string {
	for _, s := range stringsValue(c.Config["Env"]) {
		if strings.HasPrefix(s, k+"=") {
			return strings.TrimPrefix(s, k+"=")
		}
	}
	return ""
}
func same(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	return reflect.DeepEqual(a, b)
}
func sameList(a, b any) bool {
	x, ok := strictStrings(a)
	if !ok {
		return false
	}
	y, ok := strictStrings(b)
	if !ok || len(x) != len(y) {
		return false
	}
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
func strictStrings(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	switch a := v.(type) {
	case []string:
		return append([]string(nil), a...), true
	case []any:
		out := []string{}
		for _, v := range a {
			s, ok := v.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

type docker struct {
	client       *http.Client
	base         string
	self         Container
	target, task string
	mu           sync.Mutex
	ids          map[string]bool
	imageConfig  map[string]any
	recordError  func(string, []byte) error
	imageID      string
}

func newDocker() *docker {
	t := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &docker{client: &http.Client{Transport: t, Timeout: 12 * time.Minute}, base: "http://docker", ids: map[string]bool{}}
}
func (d *docker) request(ctx context.Context, method, p string, body []byte) ([]byte, error) {
	if !d.allowed(method, p, body) {
		return nil, ErrUnsafe
	}
	u, _ := url.ParseRequestURI(p)
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if (method == "POST" || method == "DELETE") && len(parts) >= 2 && parts[0] == "containers" && parts[1] != "create" {
		c, e := d.inspect(ctx, parts[1])
		if e != nil {
			return nil, e
		}
		l := labels(c)
		if !(l["com.docker.compose.project"] == d.project() && l["com.docker.compose.service"] == "app") && !(l[label] == d.task && l["io.mediatool.project"] == d.project()) {
			return nil, ErrUnsafe
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, d.base+apiVersion+p, bytes.NewReader(body))
	if e != nil {
		return nil, ErrUnsafe
	}
	req.Header.Set("Content-Type", "application/json")
	r, e := d.client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("Docker 连接不可用: %w", e)
	}
	defer deferutil.Close(r.Body, "Descriptor teardown only; primary operation and persistence results are checked")
	b, e := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if e != nil {
		return nil, e
	}
	if r.StatusCode >= 300 {
		if d.recordError != nil {
			stage := "docker"
			if len(parts) > 2 {
				stage += "_" + parts[len(parts)-1]
			} else {
				stage += "_request"
			}
			if e := d.recordError(stage, b); e != nil {
				return nil, e
			}
		}
		return nil, dockerError{r.StatusCode}
	}
	if method == "GET" && strings.HasPrefix(p, "/images/") {
		var image struct {
			ID     string `json:"Id"`
			Config map[string]any
		}
		if json.Unmarshal(b, &image) == nil {
			d.mu.Lock()
			d.imageConfig = image.Config
			d.imageID = image.ID
			d.mu.Unlock()
		}
	}
	if method == "POST" && strings.HasPrefix(p, "/containers/create?") {
		var v struct {
			ID string `json:"Id"`
		}
		if json.Unmarshal(b, &v) == nil && containerID.MatchString(v.ID) {
			d.mu.Lock()
			d.ids[v.ID] = true
			d.mu.Unlock()
		}
	}
	if method == "POST" && strings.HasPrefix(p, "/images/create?") {
		for _, line := range bytes.Split(b, []byte("\n")) {
			var v struct{ Error string }
			if json.Unmarshal(line, &v) == nil && v.Error != "" {
				return nil, fmt.Errorf("镜像拉取失败")
			}
		}
	}
	return b, nil
}
func (d *docker) inspect(ctx context.Context, id string) (Container, error) {
	var c Container
	b, e := d.request(ctx, "GET", "/containers/"+url.PathEscape(id)+"/json", nil)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func (d *docker) project() string { return labels(d.self)["com.docker.compose.project"] }
func (d *docker) appName() string { return strings.TrimPrefix(d.self.Name, "/") }
func (d *docker) scoped(id string) bool {
	if id == d.self.ID || id == d.appName() || id == d.helperName() || d.validAppName(id) {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ids[id]
}
func (d *docker) retainedName() string { return d.appName() + "-rollback-" + d.task }
func (d *docker) helperName() string   { return d.project() + "-updater-" + d.task }
func (d *docker) allowed(method, p string, b []byte) bool {
	u, e := url.ParseRequestURI(p)
	if e != nil || u.Fragment != "" || (strings.Contains(u.EscapedPath(), "%") && !strings.HasPrefix(u.Path, "/images/")) || path.Clean(u.Path) != u.Path {
		return false
	}
	s := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if d.self.ID == "" {
		return method == "GET" && len(s) == 3 && s[0] == "containers" && containerID.MatchString(s[1]) && s[2] == "json"
	}
	switch {
	case (method == "GET" || method == "HEAD") && (u.Path == "/_ping" || u.Path == "/version"):
		return true
	case method == "POST" && u.Path == "/images/create":
		return len(u.Query()) == 2 && u.Query().Get("platform") == "linux/amd64" && u.Query().Get("fromImage") == d.target && validImage(d.target)
	case method == "GET" && len(s) >= 3 && s[0] == "images" && s[len(s)-1] == "json":
		id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/images/"), "/json")
		d.mu.Lock()
		imageID := d.imageID
		d.mu.Unlock()
		return id == d.target || id == d.self.Image || id == imageID || id == fmt.Sprint(d.self.Config["Image"])
	case method == "GET" && u.Path == "/containers/json":
		return scopedFilter(u.Query().Get("filters"), d.project())
	case method == "GET" && u.Path == "/networks":
		return scopedFilter(u.Query().Get("filters"), d.project())
	case method == "GET" && len(s) == 2 && s[0] == "networks":
		_, ok := d.self.NetworkSettings.Networks[s[1]]
		return ok
	case method == "GET" && len(s) == 2 && s[0] == "volumes":
		for _, m := range d.self.Mounts {
			if m.Type == "volume" && m.Name == s[1] {
				return true
			}
		}
		return false
	case method == "POST" && u.Path == "/containers/create":
		return d.validCreate(u.Query().Get("name"), b)
	case len(s) >= 2 && s[0] == "containers" && d.scoped(s[1]):
		if len(s) == 3 {
			switch s[2] {
			case "json":
				return method == "GET"
			case "start", "stop", "wait":
				return method == "POST"
			case "rename":
				return method == "POST" && d.validAppName(u.Query().Get("name"))
			}
		}
		return len(s) == 2 && method == "DELETE" && u.Query().Get("v") != "1" && u.Query().Get("v") != "true"
	}
	return false
}
func urlQuery(s string) string { return url.QueryEscape(s) }
func has(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func validImage(s string) bool {
	return regexp.MustCompile(`^dingding229/mediatool@sha256:[a-f0-9]{64}$`).MatchString(s)
}
func (d *docker) validAppName(n string) bool {
	if n == d.retainedName() {
		return true
	}
	if n == d.appName() || n == d.self.ID[:12]+"_"+d.appName() || n == d.self.ID+"_"+d.appName() {
		return true
	}
	// During rollback Compose's temporary replacement is prefixed by the NEW
	// app ID. Only IDs created or freshly re-verified for this app may be used.
	d.mu.Lock()
	defer d.mu.Unlock()
	for id := range d.ids {
		if containerID.MatchString(id) && (n == id[:12]+"_"+d.appName() || n == id+"_"+d.appName()) {
			return true
		}
	}
	return false
}
func (d *docker) validCreate(name string, b []byte) bool {
	var c map[string]any
	if json.Unmarshal(b, &c) != nil {
		return false
	}
	if name == d.helperName() {
		want, e := json.Marshal(d.helperConfig())
		if e != nil {
			return false
		}
		var v any
		if json.Unmarshal(want, &v) != nil {
			return false
		}
		return reflect.DeepEqual(c, v)
	}
	if !d.validAppName(name) || c["Image"] != d.target {
		return false
	}
	l, _ := c["Labels"].(map[string]any)
	if l["com.docker.compose.project"] != d.project() || l["com.docker.compose.service"] != "app" {
		return false
	}
	for k, v := range c {
		switch k {
		case "Image", "HostConfig", "NetworkingConfig", "Labels":
			continue
		case "Hostname":
			if v == "" && d.self.Config[k] == d.self.ID[:12] {
				continue
			}
		}
		if !same(v, d.self.Config[k]) {
			return false
		}
	}
	d.mu.Lock()
	base := d.imageConfig
	d.mu.Unlock()
	oldLabels := labels(d.self)
	for k, v := range l {
		want := any(oldLabels[k])
		if strings.HasPrefix(k, "org.opencontainers.image.") {
			if bl, ok := base["Labels"].(map[string]any); ok && bl[k] != nil {
				want = bl[k]
			}
		}
		if v != want {
			return false
		}
	}
	for k := range oldLabels {
		if _, ok := l[k]; !ok {
			return false
		}
	}
	effective := map[string]any{}
	for k, v := range c {
		effective[k] = v
	}
	for _, k := range []string{"User", "Entrypoint", "Cmd", "WorkingDir", "ExposedPorts", "Healthcheck", "StopSignal", "Volumes"} {
		if effective[k] == nil || effective[k] == "" {
			effective[k] = base[k]
		}
	}
	for _, k := range []string{"User", "Entrypoint", "Cmd", "WorkingDir", "ExposedPorts", "Healthcheck", "StopSignal", "StopTimeout", "Tty", "OpenStdin", "Volumes"} {
		if !same(effective[k], d.self.Config[k]) {
			if (effective[k] == nil || effective[k] == false || effective[k] == "") && (d.self.Config[k] == nil || d.self.Config[k] == false || d.self.Config[k] == "") {
				continue
			}
			return false
		}
	}
	if _, ok := strictStrings(c["Env"]); !ok {
		return false
	}
	if !sameList(mergedEnv(base["Env"], c["Env"]), d.self.Config["Env"]) {
		return false
	}
	h, _ := c["HostConfig"].(map[string]any)
	if h == nil {
		return false
	}
	originalMounts, e1 := mountSpecs(d.self.HostConfig)
	requestedMounts, e2 := mountSpecs(h)
	if e1 != nil || e2 != nil || !same(originalMounts, requestedMounts) {
		return false
	}
	for _, k := range []string{"Binds", "CapAdd", "CapDrop", "SecurityOpt", "GroupAdd", "VolumesFrom", "ExtraHosts", "Dns", "Devices", "DeviceRequests", "Mounts", "PortBindings", "Privileged", "ReadonlyRootfs", "PidMode", "IpcMode", "UTSMode", "UsernsMode", "NetworkMode", "Tmpfs", "Sysctls", "AutoRemove", "PublishAllPorts"} {
		if k == "Binds" || k == "Mounts" {
			continue
		}
		if !same(h[k], d.self.HostConfig[k]) {
			if k == "IpcMode" && (h[k] == nil || h[k] == "") && d.self.HostConfig[k] == "private" {
				continue
			}
			if sameList(h[k], d.self.HostConfig[k]) {
				switch k {
				case "Binds", "CapAdd", "CapDrop", "SecurityOpt", "GroupAdd", "ExtraHosts", "VolumesFrom", "Dns":
					continue
				}
			}
			if (h[k] == nil || h[k] == false || h[k] == "") && (d.self.HostConfig[k] == nil || d.self.HostConfig[k] == false || d.self.HostConfig[k] == "") {
				continue
			}
			return false
		}
	}
	// Every additional HostConfig value must retain the old value. Defaults below
	// are daemon defaults, not a permission to supply a different explicit value.
	for k, v := range h {
		if k == "Binds" || k == "Mounts" {
			continue
		}
		old := d.self.HostConfig[k]
		if same(v, old) {
			continue
		}
		if v == nil {
			continue
		}
		if k == "ShmSize" && v == float64(0) && old == float64(67108864) {
			continue
		}
		if (k == "IpcMode" || k == "CgroupnsMode") && v == "" && old == "private" {
			continue
		}
		if k == "LogConfig" && defaultLog(v, old) {
			continue
		}
		if k == "RestartPolicy" && defaultRestart(v, old) {
			continue
		}
		if sameList(v, old) {
			switch k {
			case "Binds", "CapAdd", "CapDrop", "SecurityOpt", "GroupAdd", "ExtraHosts", "VolumesFrom", "Dns":
				continue
			}
		}
		if (v == nil || v == false || v == "") && (old == nil || old == false || old == "") {
			continue
		}
		return false
	}
	if netcfg, ok := c["NetworkingConfig"].(map[string]any); ok {
		ends, _ := netcfg["EndpointsConfig"].(map[string]any)
		if len(ends) != len(d.self.NetworkSettings.Networks) {
			return false
		}
		for n, v := range ends {
			if _, exists := d.self.NetworkSettings.Networks[n]; !exists {
				return false
			}
			ep, _ := v.(map[string]any)
			for _, k := range []string{"IPAMConfig", "Links", "DriverOpts"} {
				if ep[k] != nil {
					return false
				}
			}
			for _, alias := range stringsValue(ep["Aliases"]) {
				if alias != "app" && !d.validAppName(alias) {
					return false
				}
			}
		}
	}

	return true
}
func (d *docker) helperConfig() map[string]any {
	var data Mount
	for _, m := range d.self.Mounts {
		if m.Destination == "/data" {
			data = m
		}
	}
	work := labels(d.self)["com.docker.compose.project.working_dir"]
	binds := []string{work + ":" + work + ":rw", socket + ":" + socket + ":rw"}
	if data.Type == "bind" {
		binds = append(binds, data.Source+":/data:rw")
	} else {
		binds = append(binds, data.Name+":/data:rw")
	}
	return map[string]any{"Image": d.target, "User": "0:0", "Entrypoint": []string{"/app/media-tool"}, "Cmd": []string{"updater", d.task}, "Labels": map[string]string{label: d.task, "io.mediatool.project": d.project()}, "HostConfig": map[string]any{"AutoRemove": false, "Binds": binds, "NetworkMode": d.project() + "_default", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "CapAdd": []string{"CHOWN", "DAC_OVERRIDE"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Tmpfs": map[string]string{"/tmp": "rw,nosuid,nodev,size=64m"}}}
}

// proxy is only used by our fixed Compose command, never exposed by the web API.
func (d *docker) proxy(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, apiVersion)
	if r.URL.RawQuery != "" {
		p += "?" + r.URL.RawQuery
	}
	if (r.Method == "GET" || r.Method == "HEAD") && p == "/_ping" {
		w.Header().Set("API-Version", "1.47")
		_, _ = w.Write([]byte("OK"))
		return
	}
	// Compose only needs daemon capabilities, not host identity or arbitrary info.
	if r.Method == "GET" && p == "/info" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64","Swarm":{"LocalNodeState":"inactive"}}`))
		return
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if e != nil {
		http.Error(w, "denied", 403)
		return
	}
	out, e := d.request(r.Context(), r.Method, p, b)
	if e != nil {
		code := 403
		if v, ok := e.(dockerError); ok {
			code = v.code
		}
		slog.Warn("Docker allowlist operation rejected", "method", r.Method, "path", r.URL.Path, "status", code)
		http.Error(w, "scoped Docker operation denied or failed", code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "POST" && strings.HasPrefix(p, "/containers/create?") {
		w.WriteHeader(201)
	}
	_, _ = w.Write(out)
}

func scopedFilter(raw, project string) bool {
	var f map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &f) != nil {
		return false
	}
	var a []string
	if json.Unmarshal(f["label"], &a) == nil {
		return has(a, "com.docker.compose.project="+project)
	}
	var m map[string]bool
	return json.Unmarshal(f["label"], &m) == nil && m["com.docker.compose.project="+project]
}

func mergedEnv(base, over any) []string {
	m := map[string]string{}
	for _, list := range [][]string{stringsValue(base), stringsValue(over)} {
		for _, v := range list {
			k, _, ok := strings.Cut(v, "=")
			if ok {
				m[k] = v
			}
		}
	}
	out := []string{}
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
func defaultLog(a, b any) bool {
	x, ok := a.(map[string]any)
	if !ok {
		return false
	}
	y, ok := b.(map[string]any)
	return ok && x["Type"] == "" && y["Type"] == "json-file" && lenMap(x["Config"]) == 0 && lenMap(y["Config"]) == 0
}
func defaultRestart(a, b any) bool {
	x, ok := a.(map[string]any)
	if !ok {
		return false
	}
	y, ok := b.(map[string]any)
	return ok && x["Name"] == "" && y["Name"] == "no" && same(x["MaximumRetryCount"], y["MaximumRetryCount"])
}
func lenMap(v any) int { m, _ := v.(map[string]any); return len(m) }
