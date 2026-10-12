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
func rulesManifest() Manifest {
	return Manifest{ID: "owned-rules", Name: "Owned rule", Version: "1", Contract: 1, Runtime: "rules", Capabilities: []string{"media.recognize"}, Rules: []NameRule{{Pattern: `^Owned\.(?P<title>.+)\.S\d+E\d+\.mkv$`, Title: "{{setting:prefix}}${title}", MediaType: "series"}}, Settings: []SettingField{{Key: "prefix", Label: "Prefix", Type: "string", Default: "Owned "}, {Key: "apiKey", Label: "Own plugin key", Type: "password"}}}
}
func TestZIPPackagesRejectUnsafeEntriesAndExecuteRules(t *testing.T) {
	m := rulesManifest()
	raw, _ := json.Marshal(m)
	p, e := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw, "README.md": []byte("owned fixture")}))
	if e != nil || p.Digest == "" {
		t.Fatal(p, e)
	}
	out, e := rulesOutput(m, Event{Kind: "media.recognize", Data: map[string]any{"filename": "Owned.Show.S01E03.mkv"}}, map[string]string{"prefix": "Test "})
	if e != nil || !bytes.Contains(out, []byte(`"title":"Test Show"`)) {
		t.Fatal(string(out), e)
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
	m.Rules[0].Title = "{{setting:apiKey}}"
	if Validate(m) == nil {
		t.Fatal("secret injected into title")
	}
}
func TestPluginSettingSchema(t *testing.T) {
	m := rulesManifest()
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
	m = rulesManifest()
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
	m := rulesManifest()
	db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, m.ID)
	defer db.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, m.ID)
	raw, _ := json.Marshal(m)
	p, e := ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw}))
	if e != nil || r.Install(ctx, p, 0) != nil {
		t.Fatal(e)
	}
	if e = r.SaveSettings(ctx, m.ID, map[string]string{"prefix": "Fixture ", "apiKey": "owned-secret-fixture"}, nil, 1); e != nil {
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
	if e = r.Save(ctx, m, true, false, rev); e != nil {
		t.Fatal(e)
	}
	out, e := r.Test(ctx, m.ID, "Owned.Show.S01E03.mkv")
	if e != nil || !bytes.Contains(out, []byte("Fixture Show")) {
		t.Fatal(string(out), e)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*)FROM extension_runs WHERE plugin_id=$1 AND state='succeeded'`, m.ID).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	m.Version = "2"
	raw, _ = json.Marshal(m)
	p, e = ParsePackage(zipFixture(t, map[string][]byte{"manifest.json": raw}))
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
	if e != nil || values["prefix"] != "Fixture " || rev != 4 {
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

func TestOutputRedactionUsesInvocationSettingsSnapshot(t *testing.T) {
	m := rulesManifest()
	raw, e := redactOutput(json.RawMessage(`{"title":"prefix-owned-secret-suffix","owned-secret":"owned-secret"}`), m, map[string]string{"apiKey": "owned-secret"})
	if e != nil || bytes.Contains(raw, []byte("owned-secret")) {
		t.Fatal(string(raw), e)
	}
}
