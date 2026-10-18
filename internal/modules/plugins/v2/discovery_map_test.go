package v2

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestF4CMapEveryProducerAndExactScope(t *testing.T) {
	root := "."
	for range 5 {
		if _, e := os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		root = filepath.Join(root, "..")
	}
	raw, e := os.ReadFile(filepath.Join(root, "testdata/contracts/plugin-v2-f4c-event-map.json"))
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		Type, Kind, Scope string
		Producer, Tests   []string
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	want := map[string]bool{"metadata.scraped": true, "subscription.coverage.completed": true, "subscribe.episodes.refresh": true, "subscribe.completion.check": true, "media.recognize.convert": true, "discover.source": true, "recommend.source": true, "metadata.identify": true}
	for _, r := range rows {
		prefix := "events.subscribe."
		if r.Kind == "chain" {
			prefix = "events.chain."
		}
		if r.Kind == "module" {
			prefix = "module."
		}
		if !want[r.Type] || r.Scope != prefix+r.Type || !contracts.KnownScope(r.Scope) || len(r.Tests) < 2 || len(r.Producer) == 0 {
			t.Fatal(r)
		}
		for _, v := range r.Producer {
			path, _, _ := strings.Cut(v, ":")
			if _, e = os.Stat(filepath.Join(root, path)); e != nil {
				t.Fatal(e)
			}
		}
		delete(want, r.Type)
	}
	if len(want) > 0 {
		t.Fatal(want)
	}
	// No new URL constructor, resolver, STRM writer or execution package may be
	// introduced by a policy extension. The closed wire DTOs enforce this at runtime.
	for _, name := range []string{"internal/modules/plugins/v2/discovery_hooks.go", "internal/modules/plugins/v2/source_hooks.go", "internal/modules/catalog/sources.go", "internal/modules/subscriptions/completion.go", "internal/modules/library/metadata_policy.go"} {
		raw, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		for _, forbidden := range []string{"ffx", "linksource", "WriteAtomic", "ResolveURL", "PlaybackURL", "StreamURL"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal(name, forbidden)
			}
		}
	}
}
