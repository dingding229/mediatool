package tooling

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportMatrixUsesExactPublicContract(t *testing.T) {
	root := project(t)
	b, e := RenderMatrix(root)
	if e != nil {
		t.Fatal(e)
	}
	old := source(t, "docs/PLUGIN_SUPPORT_MATRIX.md")
	for _, s := range []string{strings.Split(strings.Split(old, "- 契约摘要：`")[1], "`")[0], "events.chain.", "**N/A：契约不接受注册**", "active=false", "contractVersion"} {
		has(t, string(b), s)
	}
	if e = matrix(root, true); e != nil {
		t.Fatal(e)
	}
}
func TestFailureRecordingDoesNotSerializeSecretPayload(t *testing.T) {
	s := source(t, "internal/platform/httpapi/plugins_v2.go")
	body := strings.SplitN(strings.SplitN(s, "func (a *API) pluginOperationRejected(", 2)[1], "\nfunc ", 2)[0]
	for _, bad := range []string{".Error()", "in.Config", "OperationKey"} {
		if strings.Contains(body, bad) {
			t.Fatal(bad)
		}
	}
	for _, name := range []string{"api.go", "external_api.go"} {
		has(t, source(t, filepath.Join("internal/platform/httpapi", name)), `"version": buildinfo.Version`)
	}
}
func TestAllLiveEntrypointsNoPython(t *testing.T) {
	root := project(t)
	paths := []string{"Makefile", ".github/workflows/build.yml"}
	files, _ := filepath.Glob(filepath.Join(root, "scripts/*.sh"))
	for _, p := range files {
		rel, _ := filepath.Rel(root, p)
		paths = append(paths, rel)
	}
	for _, p := range paths {
		b, e := os.ReadFile(filepath.Join(root, p))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(strings.ToLower(string(b)), "python") {
			t.Fatal("live Python entry", p)
		}
	}
}
