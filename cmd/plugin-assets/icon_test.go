package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIconRejectsRemoteActiveAndEscapingResources(t *testing.T) {
	for _, text := range []string{`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`, `<?xml-stylesheet href="https://example.invalid/x"?><svg/>`, `<!DOCTYPE svg><svg/>`, `<svg><image href="https://example.invalid/x"/></svg>`, `<svg onload="alert(1)"/>`, `<svg><path fill="url(https://example.invalid/x)"/></svg>`} {
		if validateIcon("icon.svg", []byte(text)) == nil {
			t.Fatal("unsafe SVG accepted", text)
		}
	}
	if validateIcon("icon.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect fill="#fff"/></svg>`)) != nil {
		t.Fatal("static icon rejected")
	}
	dir := t.TempDir()
	plugin := filepath.Join(dir, "plugins", "fixture")
	os.MkdirAll(plugin, 0755)
	os.WriteFile(filepath.Join(plugin, "icon.svg"), []byte(`<svg/>`), 0644)
	os.Symlink("icon.svg", filepath.Join(plugin, "link.svg"))
	if _, e := exportIcon(dir, "fixture", "link.svg", t.TempDir(), false); e == nil {
		t.Fatal("symlink icon exported")
	}
	if _, e := exportIcon(dir, "fixture", "../../escape.svg", t.TempDir(), false); e == nil {
		t.Fatal("path escape allowed")
	}
	if _, e := exportIcon(dir, "fixture", "icon.svg", t.TempDir(), false); e != nil {
		t.Fatal(e)
	}
}
