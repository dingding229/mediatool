package embyapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/nativelibrary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeNativeRoutesMatchOpenAPI(t *testing.T) {
	server, e := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settingsFixture(), nil })})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Listener string `json:"x-listener"`
		}
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	actual, documented := map[string]bool{}, map[string]bool{}
	for _, r := range server.router.routes {
		p := strings.ReplaceAll(r.Pattern, "*", "{wildcard}")
		actual[strings.ToLower(r.Method)+" "+p] = true
		if r.Method == "GET" {
			actual["head "+p] = true
		}
		actual["options "+p] = true
	}
	for p, methods := range doc.Paths {
		for m, op := range methods {
			if op.Listener == "native-media" {
				documented[m+" "+p] = true
			}
		}
	}
	if !reflect.DeepEqual(actual, documented) {
		for k := range actual {
			if !documented[k] {
				t.Error("missing", k)
			}
		}
		for k := range documented {
			if !actual[k] {
				t.Error("extra", k)
			}
		}
	}
	t.Logf("native runtime registrations=%d, expanded operations=%d", len(server.router.routes), len(actual))
}
