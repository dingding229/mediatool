package mediaserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProviderBoundaryHonestCapabilities(t *testing.T) {
	b := Builtin{}
	r, err := NewRegistry(b)
	if err != nil {
		t.Fatal(err)
	}
	descriptions := r.Descriptions()
	if len(descriptions) != 1 || descriptions[0].Contract != "v1" || descriptions[0].Runtime != "go-embedded" || !descriptions[0].Capabilities.MemberAuth || descriptions[0].Capabilities.PlaybackTickets || descriptions[0].Capabilities.Progress {
		t.Fatal(descriptions)
	}
	if _, err = NewRegistry(b, b); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate provider")
	}
	if _, err = r.Provider("arbitrary-url"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown provider")
	}
	raw, _ := json.Marshal(Item{ID: "asset", Title: "Movie"})
	for _, field := range []string{"filePath", "pickCode", "apiKey", "token", "grant"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("sensitive DTO field")
		}
	}
}
