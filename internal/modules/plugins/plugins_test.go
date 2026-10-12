package plugins

import (
	"context"
	"testing"
)

func TestCoreFeaturesAreNotPluginsAndLegacyRuntimesAreRejected(t *testing.T) {
	m := Manifest{ID: "owned-helper", Name: "Helper", Version: "1", Contract: 1, Runtime: "wasm", Capabilities: []string{"media.recognize"}}
	if Validate(m) != nil {
		t.Fatal(m)
	}
	for _, runtime := range []string{"", "rules", "webhook", "python", "shell"} {
		m.Runtime = runtime
		if Validate(m) == nil {
			t.Fatal("non-sandbox runtime accepted", runtime)
		}
	}
	m.Runtime = "wasm"
	for _, id := range []string{"release-name", "naming-template"} {
		m.ID = id
		if Validate(m) == nil {
			t.Fatal("core ID used by plugin", id)
		}
	}
	items, err := (Registry{}).List(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatal("core features in plugin list", items, err)
	}
	for _, sample := range []struct{ name, title, kind string }{
		{"Movie.2024.1080p.WEB-DL-GRP.mkv", "Movie", "movie"},
		{"Owned.Show.2026.S01E03.1080p.WEB-DL.x265-TEAM.mkv", "Owned Show", "series"},
	} {
		hint, msg := (Registry{}).Recognize(context.Background(), sample.name)
		if hint.Title != sample.title || hint.MediaType != sample.kind || len(msg) != 0 {
			t.Fatal(hint, msg)
		}
	}
}
