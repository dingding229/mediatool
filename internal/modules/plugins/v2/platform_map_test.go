package v2

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestF4DMapEveryProducerAndExactScope(t *testing.T) {
	root := "."
	for range 5 {
		if _, e := os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		root = filepath.Join(root, "..")
	}
	raw, e := os.ReadFile(filepath.Join(root, "testdata/contracts/plugin-v2-f4d-event-map.json"))
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
	want := map[string]bool{"notification.queued": true, "system.operation.failed": true, "settings.changed": true, "plugin.config.changed": true, "messaging.inbound": true, "messaging.action.received": true, "notifications.channel": true, "mediaserver.refresh": true}
	for _, r := range rows {
		prefix := "events.subscribe."
		if r.Kind == "module" {
			prefix = "module."
		}
		if !want[r.Type] || r.Scope != prefix+r.Type || !contracts.KnownScope(r.Scope) || len(r.Tests) < 2 {
			t.Fatal(r)
		}
		for _, path := range r.Producer {
			p, _, _ := strings.Cut(path, ":")
			if _, e = os.Stat(filepath.Join(root, p)); e != nil {
				t.Fatal(e)
			}
		}
		delete(want, r.Type)
	}
	if len(want) != 0 {
		t.Fatal(want)
	}
}
