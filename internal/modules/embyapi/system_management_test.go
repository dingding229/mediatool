package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAuthenticatedSystemInfoUsesSettingsGolden(t *testing.T) {
	value := settingsFixture()
	s, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }), Auth: &fakeAccountService{user: contracts.User{ID: "synthetic-user", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/contracts/native-system-info.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/System/Info", "/emby/sYsTeM/iNfO", "/mediabrowser/System/Info"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var got, want any
		if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		a, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || !bytes.Equal(a, b) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	value.ServerName = "Changed Server"
	r := httptest.NewRequest("GET", "/System/Info", nil)
	r.Header.Set("X-Emby-Token", "valid-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ServerName":"Changed Server"`)) {
		t.Fatal("stale server name", w.Body.String())
	}
}
