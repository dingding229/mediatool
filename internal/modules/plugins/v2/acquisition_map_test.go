package v2

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestF4AEventMapHasEveryProducerSchemaScopeAndTests(t *testing.T) {
	// go test uses the package directory; the non-root static kernel gate runs
	// this same test executable from the repository root. Locate the checkout
	// without making any fixture/world-permission or production sandbox changes.
	root := "."
	found := false
	for range 5 {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			found = true
			break
		}
		root = filepath.Join(root, "..")
	}
	if !found {
		t.Fatal("repository fixture root unavailable")
	}
	raw, e := os.ReadFile(filepath.Join(root, "testdata/contracts/plugin-v2-f4a-event-map.json"))
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		Type, Kind, Phase, State, Scope, Schema string
		Producer, Tests                         []string
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	want := map[string]bool{"site.account.updated": true, "site.account.deleted": true, "site.metrics.refreshed": true, "subscription.created": true, "subscription.updated": true, "subscription.deleted": true, "resource.selection": true, "resource.download": true}
	for _, r := range rows {
		if !want[r.Type] || r.Phase != "F4-A" || r.State != "implemented" || len(r.Producer) == 0 || len(r.Tests) < 2 || !strings.HasPrefix(r.Schema, "sdk/contracts/acquisition.go:") {
			t.Fatal(r)
		}
		prefix := "events.subscribe."
		if r.Kind == "chain" {
			prefix = "events.chain."
		}
		if r.Scope != prefix+r.Type || !contracts.KnownScope(r.Scope) {
			t.Fatal(r.Scope)
		}
		for _, producer := range r.Producer {
			path, _, ok := strings.Cut(producer, ":")
			if !ok {
				t.Fatal(producer)
			}
			if _, e = os.Stat(filepath.Join(root, path)); e != nil {
				t.Fatal(e)
			}
		}
		delete(want, r.Type)
	}
	if len(want) != 0 {
		t.Fatal("missing F4-A", want)
	}
}
