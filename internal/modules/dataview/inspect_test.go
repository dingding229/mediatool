package dataview

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNestedCredentialsAndSignedURLsRedacted(t *testing.T) {
	v := Redact("metadata", map[string]any{"title": "owned film", "password": "hidden-pw", "nested": `{"cookie":"private-cookie","url":"https://cdn.example/a?apiKey=hidden-key"}`, "binary": []byte("encrypted")})
	raw, _ := json.Marshal(v)
	s := string(raw)
	for _, private := range []string{"hidden-pw", "private-cookie", "hidden-key", "encrypted"} {
		if strings.Contains(s, private) {
			t.Fatal("leak", s)
		}
	}
	if !strings.Contains(s, "owned film") {
		t.Fatal(s)
	}
}
