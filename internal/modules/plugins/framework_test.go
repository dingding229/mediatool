package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
	"time"
)

func wasmFixture(t *testing.T) Package {
	t.Helper()
	raw, e := os.ReadFile("../../../examples/plugins/automation.zip")
	if e != nil {
		t.Fatal(e)
	}
	p, e := ParsePackage(raw)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRealGoWASISandboxRunsAndHasNoFilesystemOrEnvironmentByDefault(t *testing.T) {
	p := wasmFixture(t)
	out, e := executeWASM(context.Background(), p.Module, p.Manifest, Event{Version: 1, Kind: "plugin.test", Settings: map[string]string{"name": "Owned"}}, json.RawMessage(`{}`), nil)
	if e != nil || !bytes.Contains(out.Result, []byte("WASM")) || !bytes.Contains(out.Result, []byte(`"filesystemDenied":true`)) || !bytes.Contains(out.Result, []byte(`"environmentDenied":true`)) {
		t.Fatal(string(out.Result), e)
	}
	p.Module = []byte("bad module")
	if ValidateModule(context.Background(), p.Module) == nil {
		t.Fatal("invalid module")
	}
}
func TestPostgresWASMFullLifecycleAndServiceRevisionFence(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB")
	}
	p := wasmFixture(t)
	db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, p.Manifest.ID)
	defer db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, p.Manifest.ID)
	vault, _ := secrets.New(strings.Repeat("8", 64))
	r := Registry{DB: db, Vault: vault}
	if e = r.Install(ctx, p, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Invoke(ctx, p.Manifest.ID, "plugin.command", map[string]any{"command": "increment"}, false); e == nil {
		t.Fatal("disabled plugin executed")
	}
	if e = r.Save(ctx, p.Manifest, true, true, 1); e != nil {
		t.Fatal(e)
	}
	state, e := r.FrameworkState(ctx, p.Manifest.ID)
	if e != nil {
		t.Fatal(e)
	}
	data := state["data"].(json.RawMessage)
	if !bytes.Contains(data, []byte(`"initialized":true`)) {
		t.Fatal(string(data))
	}
	if _, e = r.Invoke(ctx, p.Manifest.ID, "plugin.command", map[string]any{"command": "increment"}, false); e != nil {
		t.Fatal(e)
	}
	if e = r.RunService(ctx, p.Manifest.ID, "counter-tick", 2); e != nil {
		t.Fatal(e)
	}
	var id string
	db.QueryRow(ctx, `SELECT id::text FROM extension_jobs WHERE plugin_id=$1 AND state='pending'`, p.Manifest.ID).Scan(&id)
	payload, _ := json.Marshal(map[string]string{"invocationId": id})
	if e = r.ProcessService(ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	if e = r.ProcessService(ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	state, _ = r.FrameworkState(ctx, p.Manifest.ID)
	if !bytes.Contains(state["data"].(json.RawMessage), []byte(`"count":2`)) {
		t.Fatal(state)
	}
	var cipher []byte
	db.QueryRow(ctx, `SELECT plugin_data_ciphertext FROM extension_plugins WHERE id=$1`, p.Manifest.ID).Scan(&cipher)
	if bytes.Contains(cipher, []byte("initialized")) {
		t.Fatal("state not encrypted")
	}
	if e = r.RunService(ctx, p.Manifest.ID, "counter-tick", 2); e != nil {
		t.Fatal(e)
	}
	if e = r.Save(ctx, p.Manifest, false, true, 2); e != nil {
		t.Fatal(e)
	}
	db.QueryRow(ctx, `SELECT id::text FROM extension_jobs WHERE plugin_id=$1 AND state='pending'`, p.Manifest.ID).Scan(&id)
	payload, _ = json.Marshal(map[string]string{"invocationId": id})
	if e = r.ProcessService(ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	var status string
	db.QueryRow(ctx, `SELECT state FROM extension_jobs WHERE id=$1`, id).Scan(&status)
	if status != "cancelled" {
		t.Fatal(status)
	}
}

func TestPostgresCloneStateAndPermissionsIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB")
	}
	p := wasmFixture(t)
	db.Exec(ctx, `DELETE FROM extension_plugins WHERE id IN('example-automation','owned-clone')`)
	defer db.Exec(ctx, `DELETE FROM extension_plugins WHERE id IN('example-automation','owned-clone')`)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	r := Registry{DB: db, Vault: vault}
	if e = r.Install(ctx, p, 0); e != nil {
		t.Fatal(e)
	}
	if e = r.Clone(ctx, p.Manifest.ID, "owned-clone", "Owned Clone", 1); e != nil {
		t.Fatal(e)
	}
	state, e := r.FrameworkState(ctx, "owned-clone")
	if e != nil || state["enabled"] != false || string(state["data"].(json.RawMessage)) != "{}" {
		t.Fatal(state, e)
	}
	if _, e = r.Invoke(ctx, "owned-clone", "plugin.command", map[string]any{"command": "increment"}, false); e == nil {
		t.Fatal("disabled clone executed")
	}
	m := p.Manifest
	m.Permissions = []string{"database.sql"}
	if Validate(m) == nil {
		t.Fatal("undeclared SQL permission accepted")
	}
}

func TestCronAndConfiguredIntervalsHaveBoundedSchedules(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	next, e := nextService(Service{Cron: "0 9 * * *"}, nil, now)
	if e != nil || !next.Equal(time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)) {
		t.Fatal(next, e)
	}
	if _, e = nextService(Service{Cron: "@every 1s"}, nil, now); e == nil {
		t.Fatal("fast cron")
	}
	next, e = nextService(Service{Cron: "* * * * *", ScheduleSetting: "cron"}, map[string]string{"cron": "*/2 * * * *"}, now)
	if e != nil || next.Sub(now) != 2*time.Minute {
		t.Fatal(next, e)
	}
}

func TestWASMHostInstantiationErrorsAreNotOverwritten(t *testing.T) {
	for _, name := range []string{"mediatool", "mediatool_host"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			runtime := wasmRuntime(ctx)
			defer runtime.Close(ctx)
			if _, err := runtime.NewHostModuleBuilder(name).Instantiate(ctx); err != nil {
				t.Fatal(err)
			}
			_, err := executeWASMRuntime(ctx, runtime, nil, Manifest{}, Event{}, nil, nil)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("host initialization error was lost: %v", err)
			}
			if name == "mediatool" && runtime.Module("mediatool_host") != nil {
				t.Fatal("second host was initialized after the first host failed")
			}
		})
	}
}
