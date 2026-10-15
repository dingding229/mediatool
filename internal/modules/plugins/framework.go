package plugins

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"media-tool/internal/modules/playback"
	"media-tool/internal/platform/trace"
)

type Command struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
type PluginRoute struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}
type Service struct {
	Cron            string `json:"cron,omitempty"`
	ScheduleSetting string `json:"scheduleSetting,omitempty"`

	ID   string `json:"id"`
	Name string `json:"name"`
}
type PluginPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type PluginLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}
type InvocationResult struct {
	Result json.RawMessage `json:"result"`
	Data   json.RawMessage `json:"data,omitempty"`
	Logs   []PluginLog     `json:"logs,omitempty"`
}

func validateFramework(m Manifest) error {
	if m.SourcePluginID != "" && !identifier.MatchString(m.SourcePluginID) {
		return ErrInvalid
	}
	if len(m.Commands) > 16 || len(m.Services) > 8 || len(m.Routes) > 16 || len(m.Pages) > 8 || len(m.Outbound) > 8 || len(m.Events) > 20 || len(m.Permissions) > 8 {
		return ErrInvalid
	}
	if m.Runtime != "wasm" {
		return ErrInvalid
	}
	permissionKeys := map[string]bool{}
	for _, p := range m.Permissions {
		if permissionKeys[p] {
			return ErrInvalid
		}
		permissionKeys[p] = true
		if !contains([]string{"library.list", "downloads.list", "tmdb.search", "notifications.send"}, p) {
			return ErrInvalid
		}
	}
	for _, event := range m.Events {
		if !contains([]string{"download.started", "download.completed", "media.imported", "media.batch_ready", "library.visible", "transfer.failed"}, event) {
			return ErrInvalid
		}
	}
	keys := map[string]bool{}
	for _, c := range m.Commands {
		if !identifier.MatchString(c.ID) || keys[c.ID] || len(c.Name) < 1 || len(c.Name) > 100 || len(c.Description) > 1000 {
			return ErrInvalid
		}
		keys[c.ID] = true
	}
	keys = map[string]bool{}
	for _, s := range m.Services {
		if !identifier.MatchString(s.ID) || keys[s.ID] || len(s.Name) < 1 || len(s.Name) > 100 || s.Cron == "" {
			return ErrInvalid
		}
		if s.Cron != "" {
			if _, e := nextService(s, nil, time.Now()); e != nil {
				return ErrInvalid
			}
		}
		if s.ScheduleSetting != "" {
			exists := false
			for _, f := range m.Settings {
				if f.Key == s.ScheduleSetting {
					exists = true
				}
			}
			if !exists {
				return ErrInvalid
			}
		}
		keys[s.ID] = true
	}
	keys = map[string]bool{}
	for _, p := range m.Pages {
		if !identifier.MatchString(p.ID) || keys[p.ID] || len(p.Title) < 1 || len(p.Title) > 100 {
			return ErrInvalid
		}
		keys[p.ID] = true
	}
	keys = map[string]bool{}
	for _, r := range m.Routes {
		if !identifier.MatchString(r.Path) || r.Method != "GET" && r.Method != "POST" || keys[r.Method+":"+r.Path] {
			return ErrInvalid
		}
		keys[r.Method+":"+r.Path] = true
	}
	keys = map[string]bool{}
	for _, origin := range m.Outbound {
		u, e := url.Parse(origin)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || keys[origin] {
			return ErrInvalid
		}
		keys[origin] = true
		if _, e = (playback.Policy{}).Syntax(origin); e != nil {
			return ErrInvalid
		}
	}
	return nil
}

type boundedBuffer struct {
	data  []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > b.limit {
		return 0, errors.New("plugin output limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func wasmRuntime(ctx context.Context) wazero.Runtime {
	return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().WithMemoryLimitPages(1024).WithCloseOnContextDone(true))
}
func ValidateModule(ctx context.Context, module []byte) error {
	if len(module) < 8 || len(module) > 16<<20 {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	runtime := wasmRuntime(ctx)
	defer runtime.Close(ctx)
	compiled, e := runtime.CompileModule(ctx, module)
	if e != nil {
		return ErrInvalid
	}
	defer compiled.Close(ctx)
	start := compiled.ExportedFunctions()["_start"]
	if start == nil || len(start.ParamTypes()) != 0 || len(start.ResultTypes()) != 0 {
		return ErrInvalid
	}
	if len(compiled.ImportedMemories()) != 0 {
		return ErrInvalid
	}
	for _, f := range compiled.ImportedFunctions() {
		ns, name, _ := f.Import()
		if ns != "wasi_snapshot_preview1" && !(ns == "mediatool" && name == "http_request") && !(ns == "mediatool_host" && name == "request") {
			return ErrInvalid
		}
	}
	return nil
}

// Go/WASI plugins run inside the app process with no mounted filesystem, no
// inherited environment or network. The single HTTP host import is scoped to
// origins explicitly declared and authorized on enable.
func executeWASM(ctx context.Context, module []byte, m Manifest, event Event, data json.RawMessage, host HostFunc) (InvocationResult, error) {
	bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	runtime := wasmRuntime(bounded)
	defer runtime.Close(context.Background())
	return executeWASMRuntime(bounded, runtime, module, m, event, data, host)
}

func executeWASMRuntime(bounded context.Context, runtime wazero.Runtime, module []byte, m Manifest, event Event, data json.RawMessage, host HostFunc) (InvocationResult, error) {
	result := InvocationResult{}
	if _, err := wasi_snapshot_preview1.Instantiate(bounded, runtime); err != nil {
		return result, err
	}
	requests := 0
	readOnly := event.Kind == "plugin.page" || event.Kind == "plugin.dashboard" || event.Kind == "plugin.test" || event.Kind == "plugin.api" && event.Data["method"] == "GET"
	_, hostError := runtime.NewHostModuleBuilder("mediatool").NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, ptr, size, out, capacity uint32) uint32 {
		requests++
		if requests > 3 {
			return 0
		}
		if size > 32<<10 || capacity > 64<<10 {
			return 0
		}
		raw, ok := mod.Memory().Read(ptr, size)
		if !ok {
			return 0
		}
		var req struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		if json.Unmarshal(raw, &req) != nil {
			return 0
		}
		u, e := url.Parse(req.URL)
		if e != nil || u.User != nil || u.Fragment != "" {
			return 0
		}
		allowed := false
		for _, origin := range m.Outbound {
			if u.Scheme+"://"+u.Host == origin {
				allowed = true
			}
		}
		if !allowed {
			return 0
		}
		policy := playback.Policy{}
		if policy.Check(ctx, req.URL) != nil {
			return 0
		}
		if req.Method == "" {
			req.Method = "GET"
		}
		if readOnly && req.Method != "GET" {
			return 0
		}
		if req.Method != "GET" && req.Method != "POST" {
			return 0
		}
		request, e := http.NewRequestWithContext(ctx, req.Method, req.URL, strings.NewReader(req.Body))
		if e != nil {
			return 0
		}
		for k, v := range req.Headers {
			if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Proxy-Authorization") || strings.ContainsAny(v, "\r\n") {
				return 0
			}
			request.Header.Set(k, v)
		}
		transport := &http.Transport{Proxy: nil, DialContext: policy.DialContext, TLSHandshakeTimeout: 2 * time.Second, IdleConnTimeout: 3 * time.Second}
		defer transport.CloseIdleConnections()
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, e := client.Do(request)
		if e != nil {
			return 0
		}
		defer response.Body.Close()
		body, e := io.ReadAll(io.LimitReader(response.Body, 60<<10))
		if e != nil {
			return 0
		}
		encoded, e := json.Marshal(map[string]any{"status": response.StatusCode, "body": string(body)})
		if e != nil || uint32(len(encoded)) > capacity {
			return 0
		}
		if !mod.Memory().Write(out, encoded) {
			return 0
		}
		return uint32(len(encoded))
	}).Export("http_request").Instantiate(bounded)
	if hostError != nil {
		return result, hostError
	}
	_, hostError = runtime.NewHostModuleBuilder("mediatool_host").NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, ptr, size, out, capacity uint32) uint32 {
		requests++
		if requests > 5 || size > 32<<10 || capacity > 128<<10 || host == nil {
			return 0
		}
		raw, ok := mod.Memory().Read(ptr, size)
		if !ok {
			return 0
		}
		var req struct {
			Operation string          `json:"operation"`
			Data      json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &req) != nil || !contains(m.Permissions, req.Operation) || readOnly && req.Operation == "notifications.send" {
			return 0
		}
		result, err := host(ctx, req.Operation, req.Data)
		var value any
		if err != nil {
			value = map[string]any{"error": err.Error()}
		} else {
			value = map[string]any{"data": result}
		}
		encoded, err := json.Marshal(value)
		if err != nil || uint32(len(encoded)) > capacity || !mod.Memory().Write(out, encoded) {
			return 0
		}
		return uint32(len(encoded))
	}).Export("request").Instantiate(bounded)
	if hostError != nil {
		return result, hostError
	}
	input, e := json.Marshal(map[string]any{"contractVersion": 1, "pluginId": m.ID, "sourcePluginId": m.SourcePluginID, "event": event, "data": data})
	if e != nil || len(input) > 300<<10 {
		return result, ErrInvalid
	}
	stdout, stderr := &boundedBuffer{limit: 256 << 10}, &boundedBuffer{limit: 16 << 10}
	compiled, e := runtime.CompileModule(bounded, module)
	if e != nil {
		return result, ErrInvalid
	}
	defer compiled.Close(context.Background())
	_, e = runtime.InstantiateModule(bounded, compiled, wazero.NewModuleConfig().WithStdin(strings.NewReader(string(input))).WithStdout(stdout).WithStderr(stderr).WithArgs("plugin").WithSysWalltime().WithSysNanotime().WithSysNanosleep().WithRandSource(rand.Reader))
	if exit, ok := e.(*sys.ExitError); ok && exit.ExitCode() == 0 {
		e = nil
	}
	if e != nil {
		trace.Warn(bounded, "plugins", "WASM运行失败", "pluginId", m.ID, "event", event.Kind, "error", e.Error(), "stderr", string(stderr.data))
		return result, errors.New("插件执行失败、超时或预算不足")
	}
	if strictJSON(stdout.data, &result) != nil || len(result.Data) > 128<<10 || len(result.Logs) > 40 {
		return result, ErrInvalid
	}
	for _, l := range result.Logs {
		if len(l.Message) > 4096 || l.Level != "debug" && l.Level != "info" && l.Level != "warn" && l.Level != "error" {
			return result, ErrInvalid
		}
	}
	return result, nil
}

func (r Registry) Invoke(ctx context.Context, id, kind string, payload map[string]any, allowDisabled bool) (json.RawMessage, error) {
	list, e := r.List(ctx)
	if e != nil {
		return nil, e
	}
	for _, item := range list {
		if item.Manifest.ID == id {
			if !item.Enabled && !allowDisabled {
				return nil, ErrInvalid
			}
			event := Event{Version: 1, Kind: kind, Data: payload}
			if !allowedEvent(item.Manifest, event) {
				return nil, ErrInvalid
			}
			return r.invokeFramework(ctx, item.Manifest, event, allowDisabled)
		}
	}
	return nil, ErrInvalid
}
func (r Registry) invokeFramework(ctx context.Context, m Manifest, event Event, allowDisabled bool) (json.RawMessage, error) {
	conn, err := r.DB.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "plugin:"+m.ID).Scan(&locked); err != nil || !locked {
		return nil, ErrInvalid
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "plugin:"+m.ID)

	var module, data, raw []byte
	var revision int64
	var enabled bool
	e := r.DB.QueryRow(ctx, `SELECT wasm_module,plugin_data_ciphertext,manifest,revision,enabled FROM extension_plugins WHERE id=$1`, m.ID).Scan(&module, &data, &raw, &revision, &enabled)
	if e != nil || !enabled && !allowDisabled {
		return nil, ErrInvalid
	}
	var current Manifest
	if json.Unmarshal(raw, &current) != nil {
		return nil, ErrInvalid
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(current)
	if string(a) != string(b) {
		return nil, ErrInvalid
	}
	opened, e := r.readData(m.ID, data)
	if e != nil {
		return nil, e
	}
	data = opened
	values, _, rev, e := r.Settings(ctx, m.ID)
	if e != nil || rev != revision {
		return nil, ErrInvalid
	}
	event.Settings = values
	trace.Info(ctx, "plugins", "插件调用开始", "pluginId", m.ID, "event", event.Kind, "revision", revision)
	result, e := executeWASM(ctx, module, m, event, data, r.Host)
	state, message := "succeeded", "插件执行完成"
	if e != nil {
		state, message = "outcome_unknown", "插件执行失败或超时；不会自动重发"
	}
	audit, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	r.recordRun(audit, m.ID, event.Kind, state, message)
	trace.Info(ctx, "plugins", "插件调用结束", "pluginId", m.ID, "event", event.Kind, "state", state)
	if e != nil {
		return nil, e
	}
	if len(result.Data) > 0 && event.Kind != "plugin.page" && event.Kind != "plugin.dashboard" && event.Kind != "plugin.test" && !(event.Kind == "plugin.api" && event.Data["method"] == "GET") {
		cipher, err := r.Vault.Seal("plugin-data:"+m.ID, map[string]string{"data": string(result.Data)})
		if err != nil {
			return nil, err
		}
		tag, e := r.DB.Exec(ctx, `UPDATE extension_plugins SET plugin_data_ciphertext=$2 WHERE id=$1 AND revision=$3 AND enabled=$4`, m.ID, cipher, revision, enabled)
		if e != nil || tag.RowsAffected() != 1 {
			return nil, ErrInvalid
		}
	}
	for _, l := range result.Logs {
		level := slog.LevelInfo
		switch l.Level {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		trace.Log(ctx, level, "plugins", l.Message, "pluginId", m.ID, "event", event.Kind)
	}
	return result.Result, nil
}
func (r Registry) FrameworkState(ctx context.Context, id string) (map[string]any, error) {
	var data []byte
	var enabled bool
	var revision int64
	e := r.DB.QueryRow(ctx, `SELECT plugin_data_ciphertext,enabled,revision FROM extension_plugins WHERE id=$1`, id).Scan(&data, &enabled, &revision)
	if e != nil {
		return nil, e
	}
	state, e := r.readData(id, data)
	if e != nil {
		return nil, e
	}
	return map[string]any{"data": state, "enabled": enabled, "revision": revision, "servicesPendingUntilEnabled": !enabled}, nil
}
func (r Registry) ResetData(ctx context.Context, id string, revision int64) error {
	tag, e := r.DB.Exec(ctx, `UPDATE extension_plugins SET plugin_data_ciphertext=NULL,enabled=false,revision=revision+1 WHERE id=$1 AND revision=$2`, id, revision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("plugin revision changed")
	}
	return nil
}

// Host is an application-supplied capability bridge, never a database/secret
// handle inside the guest. Unknown operations fail rather than bypass controls.
type HostFunc func(context.Context, string, json.RawMessage) (any, error)

func (r Registry) readData(id string, cipher []byte) (json.RawMessage, error) {
	if len(cipher) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if r.Vault == nil {
		return nil, ErrInvalid
	}
	value, e := r.Vault.Open("plugin-data:"+id, cipher)
	if e != nil {
		return nil, e
	}
	if !json.Valid([]byte(value["data"])) {
		return nil, ErrInvalid
	}
	return json.RawMessage(value["data"]), nil
}

func (r Registry) Clone(ctx context.Context, id, newID, name string, revision int64) error {
	if !identifier.MatchString(newID) || id == newID || len(name) < 1 || len(name) > 120 {
		return ErrInvalid
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var raw, cipher, module, files []byte
	var digest, readme, ui string
	e = tx.QueryRow(ctx, `SELECT manifest,config_ciphertext,wasm_module,package_files,package_digest,readme,ui_html FROM extension_plugins WHERE id=$1 AND revision=$2 FOR SHARE`, id, revision).Scan(&raw, &cipher, &module, &files, &digest, &readme, &ui)
	if e != nil {
		return ErrInvalid
	}
	var m Manifest
	if json.Unmarshal(raw, &m) != nil {
		return ErrInvalid
	}
	m.SourcePluginID = id
	m.ID = newID
	m.Name = name
	if Validate(m) != nil {
		return ErrInvalid
	}
	values := map[string]string{}
	if len(cipher) > 0 {
		values, e = r.Vault.Open("plugin:"+id, cipher)
		if e != nil {
			return e
		}
	}
	encrypted, e := r.Vault.Seal("plugin:"+newID, values)
	if e != nil {
		return e
	}
	manifest, _ := json.Marshal(m)
	_, e = tx.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest,config_ciphertext,package_digest,package_files,wasm_module,readme,ui_html)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, newID, name, m.Version, manifest, encrypted, digest, files, module, readme, ui)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func allowedEvent(m Manifest, event Event) bool {
	switch event.Kind {
	case "plugin.init", "plugin.stop", "plugin.test":
		return true
	case "plugin.command":
		for _, c := range m.Commands {
			if c.ID == event.Data["command"] {
				return true
			}
		}
	case "plugin.service":
		for _, s := range m.Services {
			if s.ID == event.Data["service"] {
				return true
			}
		}
	case "plugin.page":
		for _, p := range m.Pages {
			if p.ID == event.Data["page"] {
				return true
			}
		}
	case "plugin.dashboard":
		return m.Dashboard
	case "plugin.api":
		for _, r := range m.Routes {
			if r.Path == event.Data["path"] && r.Method == event.Data["method"] {
				return true
			}
		}
	default:
		return contains(m.Events, event.Kind) || contains(m.Capabilities, event.Kind)
	}
	return false
}
