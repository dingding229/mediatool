package embyapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

type fakeAccountService struct {
	user      contracts.User
	err       error
	mu        sync.Mutex
	seen      RequestContext
	authCalls atomic.Int32
}

func (f *fakeAccountService) Authenticate(_ context.Context, raw string, _ contracts.DeviceInfo) (contracts.User, contracts.Device, contracts.Token, error) {
	f.authCalls.Add(1)
	if raw != "valid-token" {
		return contracts.User{}, contracts.Device{}, contracts.Token{}, contracts.ErrUnauthenticated
	}
	if f.err != nil {
		return contracts.User{}, contracts.Device{}, contracts.Token{}, f.err
	}
	return f.user, contracts.Device{ID: "device-id", UserID: f.user.ID}, contracts.Token{Digest: sha256.Sum256([]byte(raw)), CredentialVersion: 1}, nil
}
func (f *fakeAccountService) Login(_ context.Context, name, password string, info contracts.DeviceInfo) (contracts.LoginResult, error) {
	if name != "synthetic.member" || password != "password123" || info.DeviceID != "player-device" {
		return contracts.LoginResult{}, contracts.ErrUnauthenticated
	}
	if f.err != nil {
		return contracts.LoginResult{}, f.err
	}
	return contracts.LoginResult{User: f.user, Device: contracts.Device{ID: "session-id", DeviceKey: info.DeviceID}, AccessToken: "valid-token"}, nil
}
func (f *fakeAccountService) GetUser(_ context.Context, id string) (contracts.User, error) {
	if id != f.user.ID {
		return contracts.User{}, contracts.ErrForbidden
	}
	return f.user, nil
}
func (f *fakeAccountService) Logout(context.Context, contracts.Token) error { return f.err }
func (f *fakeAccountService) ChangePassword(_ context.Context, u contracts.User, token contracts.Token, current, next string) error {
	if current != "password123" || next != "newpass123" || token.Digest == ([32]byte{}) || u.ID != f.user.ID {
		return contracts.ErrUnauthenticated
	}
	return f.err
}
func (f *fakeAccountService) PublicUsers(context.Context) ([]contracts.PublicUser, error) {
	return []contracts.PublicUser{{Id: f.user.ID, Name: f.user.Name, HasPassword: true}}, nil
}
func authTestServer(t *testing.T, f *fakeAccountService, images ImageResolver, logs io.Writer) *Server {
	t.Helper()
	if logs == nil {
		logs = io.Discard
	}
	value := settingsFixture()
	value.PublicUserList = true
	s, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }), Auth: f, Browse: &browseStub{}, Images: images, Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestNativeLoginBodyFormatsAndStrictResponse(t *testing.T) {
	user := contracts.User{ID: strings.Repeat("a", 32), Name: "synthetic.member", Enabled: true, MaxStreams: 2}
	accounts := &fakeAccountService{user: user}
	var logs bytes.Buffer
	s := authTestServer(t, accounts, nil, &logs)
	for _, body := range []string{`{"Username":"synthetic.member","Pw":"password123"}`, `{"username":"synthetic.member","password":"password123","unknownClientField":true}`, `{"USERNAME":"synthetic.member","PW":"password123"}`} {
		req := httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(body))
		req.Header.Set("Authorization", `MediaBrowser Client="Infuse", Device="Synthetic iPad", DeviceId="player-device", Version="8.0"`)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal("valid client login rejected", w.Code)
		}
		var got dto.AuthenticationResult
		decoder := json.NewDecoder(w.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.AccessToken != "valid-token" || got.User.Id != user.ID || got.SessionInfo == nil || got.User.Policy.IsAdministrator || got.ServerId != settingsFixture().ServerID {
			t.Fatal("authentication contract changed")
		}
	}
	for _, body := range []string{`{"Username":"synthetic.member","Pw":"password123","Password":"conflict"}`, `{"Username":1}`, `{} {}`, strings.Repeat("x", 65537)} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal("invalid login body accepted", w.Code)
		}
	}
	accounts.err = contracts.ErrPasswordChangeRequired
	req := httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(`{"Username":"synthetic.member","Pw":"password123"}`))
	req.Header.Set("X-Emby-Authorization", `Client="SenPlayer", DeviceId="player-device"`)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 401 || !strings.Contains(w.Body.String(), "管理员") {
		t.Fatal("missing readable password change response")
	}
	if strings.Contains(logs.String(), "valid-token") || strings.Contains(logs.String(), "password123") {
		t.Fatal("credential leaked into log")
	}
}
func TestVerifiedContextAndCrossUserGate(t *testing.T) {
	user := contracts.User{ID: strings.Repeat("a", 32), Name: "synthetic.member", Enabled: true, CredentialVersion: 1}
	accounts := &fakeAccountService{user: user}
	s := authTestServer(t, accounts, imageResolverFunc(func(ctx context.Context, _ ImageRequest) (ImageResult, error) {
		info, ok := RequestInfo(ctx)
		if !ok || info.User == nil || info.User.ID != user.ID || info.Device == nil || info.Token == nil {
			t.Error("verified context missing")
		}
		return ImageResult{Body: []byte("image"), ContentType: "image/jpeg", Tag: "synthetic-tag"}, nil
	}), nil)
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/Users/" + user.ID, "", 401}, {"/Users/" + user.ID, "invalid-token", 401}, {"/Users/" + strings.Repeat("b", 32), "valid-token", 403}, {"/Users/" + user.ID, "valid-token", 200}, {"/Users/" + user.ID + "/Views", "valid-token", 200}, {"/Items/100000/Images/Primary", "valid-token", 200}, {"/Items?UserId=" + strings.Repeat("b", 32), "valid-token", 403}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.token != "" {
			r.Header.Set("X-Emby-Token", tc.token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc.path, w.Code, tc.status)
		}
	}
}
func TestAnonymousImagesLimitAndVersionedCachePolicy(t *testing.T) {
	user := contracts.User{ID: strings.Repeat("a", 32), Name: "synthetic.member", Enabled: true}
	accounts := &fakeAccountService{user: user}
	tag := nativelibrary.ImageTag("/synthetic.jpg")
	calls := 0
	s := authTestServer(t, accounts, imageResolverFunc(func(_ context.Context, r ImageRequest) (ImageResult, error) {
		calls++
		return ImageResult{Body: []byte("image"), ContentType: "image/jpeg", Tag: tag}, nil
	}), nil)
	call := func(ip, query, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/Items/100000/Images/Primary"+query, nil)
		r.RemoteAddr = ip + ":1234"
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		query string
		want  string
	}{{"", "public, max-age=3600"}, {"?tag=old", "public, max-age=3600"}, {"?TaG=" + tag, "public, max-age=31536000, immutable"}} {
		w := call("93.184.216.34", tc.query, "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != tc.want {
			t.Fatal(w.Code, w.Header().Get("Cache-Control"))
		}
	}
	for i := 3; i < 600; i++ {
		if call("93.184.216.34", "", "").Code != 200 {
			t.Fatal("limited before 600")
		}
	}
	before := calls
	w := call("93.184.216.34", "", "")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || calls != before {
		t.Fatal("anonymous quota not enforced before resolver")
	}
	spoof := httptest.NewRequest("GET", "/Items/100000/Images/Primary", nil)
	spoof.RemoteAddr = "93.184.216.34:1234"
	spoof.Header.Set("X-Forwarded-For", "1.1.1.1")
	spoofed := httptest.NewRecorder()
	s.ServeHTTP(spoofed, spoof)
	if spoofed.Code != 429 {
		t.Fatal("untrusted forwarded IP bypassed the quota")
	}
	if call("93.184.216.34", "", "invalid-token").Code != 429 {
		t.Fatal("unverified token bypassed quota")
	}
	if call("93.184.216.34", "", "valid-token").Code != 200 {
		t.Fatal("valid token should bypass anonymous quota")
	}
	if call("8.8.8.8", "", "").Code != 200 {
		t.Fatal("per-IP counters were shared")
	}
}
func TestImageLimiterIsBoundedConcurrentAndRecoversWindow(t *testing.T) {
	var limiter imageLimiter
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	var admitted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 1000; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if limiter.allow("one-ip", now) {
				admitted.Add(1)
			}
		}()
	}
	group.Wait()
	if admitted.Load() != 600 {
		t.Fatal(admitted.Load())
	}
	if !limiter.allow("one-ip", now.Add(time.Minute)) {
		t.Fatal("quota did not reset")
	}
	limiter.windows = map[string]imageWindow{}
	limiter.swept = now
	for i := 0; i < 10000; i++ {
		limiter.windows[strconv.Itoa(i)] = imageWindow{start: now, count: 1}
	}
	if limiter.allow("overflow", now) {
		t.Fatal("limiter table was unbounded")
	}
	if !limiter.allow("overflow", now.Add(time.Minute)) {
		t.Fatal("expired buckets did not clear")
	}
}
