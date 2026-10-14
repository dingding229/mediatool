package mediaserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProviderBoundaryHonestCapabilities(t *testing.T) {
	b := Builtin{}
	e := EmbyAdapter{}
	r, err := NewRegistry(b, e)
	if err != nil {
		t.Fatal(err)
	}
	descriptions := r.Descriptions()
	if len(descriptions) != 2 || descriptions[0].Contract != "v1" || descriptions[0].Runtime != "go-embedded" || !descriptions[0].Capabilities.MemberAuth || descriptions[0].Capabilities.PlaybackTickets || descriptions[0].Capabilities.Progress {
		t.Fatal(descriptions)
	}
	if _, err = NewRegistry(b, b); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate provider")
	}
	if _, err = r.Provider("arbitrary-url"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown provider")
	}
	if _, err = e.List(context.Background(), Access{UserID: "member"}, Query{}); !errors.Is(err, ErrDenied) {
		t.Fatal("member received Emby administrator credentials")
	}
	if _, err = e.Playback(context.Background(), Access{}, PlaybackRequest{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unexpected legacy playback")
	}
	raw, _ := json.Marshal(Item{ID: "asset", Title: "Movie"})
	for _, field := range []string{"filePath", "pickCode", "apiKey", "token", "grant"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("sensitive DTO field")
		}
	}
}
