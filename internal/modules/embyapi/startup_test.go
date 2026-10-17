package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestStartupDefaultsGoldenAndRESTAfterSocketRefusal(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/native-startup.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Endpoint      dto.EndpointInfo          `json:"endpoint"`
		Branding      dto.BrandingConfiguration `json:"branding"`
		Display       dto.DisplayPreferences    `json:"display"`
		Localization  []struct{}                `json:"localization"`
		Sessions      []dto.SessionInfo         `json:"sessions"`
		Configuration dto.UserConfiguration     `json:"configuration"`
		Socket        json.RawMessage           `json:"socket"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	value := settingsFixture()
	id := strings.Repeat("a", 32)
	s, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }), Auth: &fakeAccountService{user: contracts.User{ID: id, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
		for _, v := range []struct {
			path, key string
			status    int
		}{{"/System/Endpoint", "endpoint", 200}, {"/Branding/Configuration", "branding", 200}, {"/DisplayPreferences", "display", 200}, {"/DisplayPreferences/usersettings?Client=Infuse", "display", 200}, {"/Localization/Cultures", "localization", 200}, {"/Localization/Countries", "localization", 200}, {"/Localization/ParentalRatings", "localization", 200}, {"/Localization/Options", "localization", 200}, {"/Sessions", "sessions", 200}, {"/Users/" + id + "/Configuration", "configuration", 200}, {"/socket", "socket", 501}} {
			r := httptest.NewRequest("GET", prefix+v.path, nil)
			r.Header.Set("X-Emby-Token", "valid-token")
			if v.key == "socket" {
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			var got, want any
			if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(fields[v.key], &want); err != nil {
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
			if w.Code != v.status || !bytes.Equal(a, b) || w.Header().Get("Upgrade") != "" {
				t.Fatal(v, w.Code, w.Body.String())
			}
		}
	}
	// Unknown locale has 404, invalid preference IDs are not reflected.
	for _, v := range []struct {
		path   string
		status int
	}{{"/Localization/not-real", 404}, {"/DisplayPreferences/" + strings.Repeat("b", 64), 400}, {"/DisplayPreferences?UserId=" + strings.Repeat("b", 32), 403}} {
		r := httptest.NewRequest("GET", v.path, nil)
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != v.status {
			t.Fatal(v, w.Code)
		}
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/Branding/Css", "/EMBY/branding/css.css"} {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 200 || w.Body.Len() != 0 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
				t.Fatal(path, w.Code)
			}
		}
	}
}
func TestUnknownEndpointDiagnosticsAreBoundedAndTokenSafe(t *testing.T) {
	var logs bytes.Buffer
	r, err := NewRouter(nil, Options{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("b", 64)
	for i := 0; i < 100; i++ {
		q := httptest.NewRequest("GET", "/unknown/"+secret+"?api_key="+secret, nil)
		q.Header.Set("User-Agent", "Infuse/8 Token="+secret)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != 404 || bytes.Contains(w.Body.Bytes(), []byte(secret)) {
			t.Fatal("unknown response")
		}
	}
	if strings.Contains(logs.String(), secret) || strings.Count(logs.String(), "endpoint not found") != 60 {
		t.Fatal("unsafe or unbounded diagnostic log")
	}
}

func TestNativeRequestBodyCapsAndFailClosedStartup(t *testing.T) {
	value := settingsFixture()
	s, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }), Auth: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/Sessions/Capabilities", "/Users/AuthenticateByName", "/Items/100001/PlaybackInfo", "/emby/Sessions/Playing"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(strings.Repeat("x", (1<<20)+1)))
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 413 || strings.Contains(w.Body.String(), "valid-token") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(`{"Username":"synthetic.member","Pw":"`+strings.Repeat("x", 65<<10)+`"}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("login body cap", w.Code)
	}
	for _, path := range []string{"/api/v1/auth/session", "/api/v1/media-server/users", "/socket/not-real"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("admin/unknown route", path, w.Code)
		}
	}
	value.Enabled = false
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/Branding/Configuration", nil))
	if w.Code != 503 {
		t.Fatal("disabled startup gate", w.Code)
	}
}

func TestPublicStartupRateAndKnownRequestConcurrencyBounds(t *testing.T) {
	value := settingsFixture()
	s := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }, "", nil)
	for i := 0; i < 601; i++ {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/System/Endpoint", nil))
		want := 200
		if i == 600 {
			want = 429
		}
		if w.Code != want {
			t.Fatal("startup throttle", i, w.Code)
		}
	}
	entered := make(chan struct{}, 64)
	release := make(chan struct{})
	slow := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) {
		entered <- struct{}{}
		<-release
		return value, nil
	}, "", nil)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Go(func() {
			w := httptest.NewRecorder()
			slow.ServeHTTP(w, httptest.NewRequest("GET", "/System/Endpoint", nil))
		})
	}
	for i := 0; i < 64; i++ {
		<-entered
	}
	w := httptest.NewRecorder()
	slow.ServeHTTP(w, httptest.NewRequest("GET", "/System/Endpoint", nil))
	if w.Code != 503 || w.Header().Get("Retry-After") != "1" {
		t.Fatal("overload not bounded", w.Code)
	}
	close(release)
	wg.Wait()
}
