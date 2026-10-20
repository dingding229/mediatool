package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type recordingMux struct {
	*http.ServeMux
	patterns []string
}

func (m *recordingMux) Handle(p string, h http.Handler) {
	m.patterns = append(m.patterns, p)
	m.ServeMux.Handle(p, h)
}
func (m *recordingMux) HandleFunc(p string, h func(http.ResponseWriter, *http.Request)) {
	m.patterns = append(m.patterns, p)
	m.ServeMux.HandleFunc(p, h)
}

// Uses the exact same constructor and registration calls as New. No AST-based
// list or manually maintained list can silently omit a dynamic registration.
func TestRuntimeManagementRegistrationsMatchOpenAPI(t *testing.T) {
	m := &recordingMux{ServeMux: http.NewServeMux()}
	_ = newWithMux(cfg(), Dependencies{}, m)
	raw, e := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Listener     string `json:"x-listener"`
			Registration string `json:"x-go-registration"`
		}
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	actual, documented := map[string]bool{}, map[string]bool{}
	for _, p := range m.patterns {
		if actual[p] {
			t.Fatal("duplicate runtime pattern", p)
		}
		actual[p] = true
	}
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.Listener == "management" {
				documented[op.Registration] = true
			}
		}
	}
	if !reflect.DeepEqual(actual, documented) {
		keys := func(m map[string]bool) []string {
			out := []string{}
			for k := range m {
				out = append(out, k)
			}
			sort.Strings(out)
			return out
		}
		for _, p := range keys(actual) {
			if !documented[p] {
				t.Error("missing", p)
			}
		}
		for _, p := range keys(documented) {
			if !actual[p] {
				t.Error("extra", p)
			}
		}
	}
	t.Logf("runtime management patterns=%d, missing/extra=0", len(actual))
}
