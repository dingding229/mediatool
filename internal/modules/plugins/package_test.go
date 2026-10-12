package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"testing"
	"time"
)

func zipFixture(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		f.Write(data)
	}
	w.Close()
	return b.Bytes()
}
func settingsManifest(t *testing.T) Manifest {
	m := wasmFixture(t).Manifest
	m.CustomUI = false
	m.Settings = []SettingField{{Key: "name", Label: "Title", Type: "string", Default: "Owned"}, {Key: "apiKey", Label: "Own plugin key", Type: "password"}}
	return m
}
func TestZIPPackagesRequireExecutableCodeAndRejectUnsafeEntries(t *testing.T) {
	m := settingsManifest(t)
	raw, _ := json.Marshal(m)
	p, e := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, "plugin.wasm": wasmFixture(t).Module, "README.md": []byte("owned fixture")}))
	if e != nil || p.Digest == "" {
		t.Fatal(p, e)
	}
	for _, runtime := range []string{"", "rules", "webhook"} {
		old := m
		old.Runtime = runtime
		manifest, _ := json.Marshal(old)
		if _, err := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": manifest, "plugin.wasm": wasmFixture(t).Module})); err == nil {
			t.Fatal("legacy runtime", runtime)
		}
	}
	for _, extra := range []string{`"endpoint":"https://owned.example"`, `"rules":[]`} {
		bad := append(append([]byte{}, raw[:len(raw)-1]...), []byte(","+extra+"}")...)
		if _, err := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": bad, "plugin.wasm": wasmFixture(t).Module})); err == nil {
			t.Fatal("legacy manifest field", extra)
		}
	}
	if _, err := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw})); err == nil {
		t.Fatal("manifest-only accepted")
	}
	for _, name := range []string{"../manifest.json", "/manifest.json", "folder/../manifest.json", "module.py", "module.wasm", "manifest.json/", "..\\manifest.json"} {
		if _, e = ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, name: []byte("bad")})); e == nil {
			t.Fatal("unsafe", name)
		}
	}
	if _, e = ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, "README.md": bytes.Repeat([]byte("a"), MaxUnpackedBytes+1)})); e == nil {
		t.Fatal("bomb")
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for range 2 {
		f, _ := w.Create("manifest.json")
		f.Write(raw)
	}
	w.Close()
	if _, e = ParsePackage(b.Bytes()); e == nil {
		t.Fatal("duplicate")
	}
	b.Reset()
	w = zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "manifest.json"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := w.CreateHeader(h)
	f.Write([]byte("/private"))
	w.Close()
	if _, e = ParsePackage(b.Bytes()); e == nil {
		t.Fatal("symlink")
	}

}
func TestPluginSettingSchema(t *testing.T) {
	m := settingsManifest(t)
	for _, v := range []map[string]string{{"unknown": "x"}, {"apiKey": string([]byte{0})}} {
		if _, e := ValidateSettings(m.Settings, v, false); e == nil {
			t.Fatal(v)
		}
	}
	if _, e := ValidateSettings([]SettingField{{Key: "active", Type: "boolean"}}, map[string]string{"active": "yes"}, false); e == nil {
		t.Fatal("boolean")
	}
	if _, e := ValidateSettings([]SettingField{{Key: "count", Type: "number"}}, map[string]string{"count": "NaN"}, false); e == nil {
		t.Fatal("NaN")
	}
	for _, field := range []SettingField{{Key: "active", Label: "Active", Type: "boolean", Default: "yes"}, {Key: "choice", Label: "Choice", Type: "select", Options: []string{"valid"}, Default: "other"}, {Key: "count", Label: "Count", Type: "number", Default: "NaN"}} {
		m.Settings = []SettingField{field}
		if Validate(m) == nil {
			t.Fatal("invalid default", field)
		}
	}
	m = settingsManifest(t)
	m.Settings[1].Default = "plaintext-secret"
	if Validate(m) == nil {
		t.Fatal("secret default")
	}
}
func TestPostgresZIPPluginLifecycleEncryptedConfig(t *testing.T) {
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
	vault, e := secrets.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if e != nil {
		t.Fatal(e)
	}
	r := Registry{DB: db, Vault: vault}
	m := settingsManifest(t)
	db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, m.ID)
	defer db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, m.ID)
	raw, _ := json.Marshal(m)
	p, e := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, "plugin.wasm": wasmFixture(t).Module}))
	if e != nil {
		t.Fatal(e)
	}
	if r.Save(ctx, m, false, false, 0) == nil {
		t.Fatal("manifest-only install accepted")
	}
	bad := p
	bad.Module = []byte("not executable WASM")
	if r.Install(ctx, bad, 0) == nil {
		t.Fatal("direct invalid-module install accepted")
	}
	if _, e = db.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest)VALUES('owned-invalid-wasm','Owned','1',$1)`, raw); e == nil {
		t.Fatal("database accepted plugin without code")
	}
	if e = r.Install(ctx, p, 0); e != nil {
		t.Fatal(e)
	}
	if e = r.SaveSettings(ctx, m.ID, map[string]string{"name": "Fixture Title", "apiKey": "owned-secret-fixture"}, nil, 1); e != nil {
		t.Fatal(e)
	}
	values, _, rev, e := r.Settings(ctx, m.ID)
	if e != nil || rev != 2 || values["apiKey"] != "owned-secret-fixture" {
		t.Fatal(values, rev, e)
	}
	var encrypted []byte
	db.QueryRow(ctx, `SELECT config_ciphertext FROM extension_plugins WHERE id=$1`, m.ID).Scan(&encrypted)
	if bytes.Contains(encrypted, []byte("owned-secret-fixture")) {
		t.Fatal("unencrypted")
	}
	if e = r.Save(ctx, m, true, true, rev); e != nil {
		t.Fatal(e)
	}
	out, e := r.Test(ctx, m.ID, "Owned.Show.S01E03.mkv")
	if e != nil || !bytes.Contains(out, []byte("WASM")) {
		t.Fatal(string(out), e)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*)FROM extension_runs WHERE plugin_id=$1 AND state='succeeded'`, m.ID).Scan(&count)
	if count != 2 {
		t.Fatal(count)
	}
	m.Version = "2"
	raw, _ = json.Marshal(m)
	p, e = ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, "plugin.wasm": wasmFixture(t).Module}))
	if e != nil || r.Install(ctx, p, 3) != nil {
		t.Fatal(e)
	}
	list, e := r.List(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range list {
		if item.Manifest.ID == m.ID && (item.Enabled || item.Manifest.Version != "2") {
			t.Fatal(item)
		}
	}
	values, _, rev, e = r.Settings(ctx, m.ID)
	if e != nil || values["name"] != "Fixture Title" || rev != 4 {
		t.Fatal(values, rev, e)
	}
	if _, e = r.Test(ctx, m.ID, "Owned.Show.S01E03.mkv"); e == nil {
		t.Fatal("disabled execution")
	}
	if e = r.SaveSettings(ctx, m.ID, map[string]string{}, []string{"apiKey"}, rev); e != nil {
		t.Fatal(e)
	}
	values, _, _, _ = r.Settings(ctx, m.ID)
	if values["apiKey"] != "" {
		t.Fatal("not cleared")
	}
	if e = r.Delete(ctx, m.ID); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Millisecond)
}
