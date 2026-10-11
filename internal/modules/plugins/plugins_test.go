package plugins

import (
	"context"
	"testing"
)

func TestManifestSafetyAndBuiltinRegistry(t *testing.T) {
	m := Manifest{ID: "my-helper", Name: "Helper", Version: "1", Contract: 1, Endpoint: "https://plugins.example/hook", Capabilities: []string{"media.recognize"}}
	if Validate(m) != nil {
		t.Fatal(m)
	}
	for _, url := range []string{"http://plugins.example", "https://127.0.0.1/hook", "https://host.local/hook", "https://user:pass@plugins.example/hook", "https://plugins.example?token=secret"} {
		m.Endpoint = url
		if Validate(m) == nil {
			t.Fatal(url)
		}
	}
	items, err := (Registry{}).List(context.Background())
	if err != nil || len(items) != 2 || !items[0].Builtin {
		t.Fatal(items, err)
	}
	hint, msg := (Registry{}).Recognize(context.Background(), "Movie.2024.1080p.WEB-DL-GRP.mkv")
	if hint.MediaType != "movie" || hint.Title != "Movie" || len(msg) != 0 {
		t.Fatal(hint, msg)
	}
}
