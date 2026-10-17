package migrations_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/nativelibrary"
)

func authFixture(t *testing.T) (nativeFixture, *nativeauth.Store, mediausers.User) {
	t.Helper()
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 58); err != nil {
		t.Fatal(err)
	}
	pool := catalogTestPool(t, f)
	user, err := (mediausers.Repository{DB: pool}).Save(f.ctx, "", mediausers.Input{Username: "synthetic.member", Password: "Original-密码123", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if user.MustChangePassword {
		t.Fatal("administrator-created password should be final by default")
	}
	return f, &nativeauth.Store{DB: pool, Secret: "synthetic-native-auth-key"}, user
}
func TestNativeAuthTokensDevicesSlidingAndImmediateInvalidation(t *testing.T) {
	f, store, adminUser := authFixture(t)
	info := contracts.DeviceInfo{DeviceID: "infuse-device", Name: "Synthetic iPad", Client: "Infuse", Version: "8.0", UserAgent: "Infuse/8.0", ClientIP: "93.184.216.34"}
	if _, err := store.Login(f.ctx, "synthetic.member", "incorrect", info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("wrong password was accepted")
	}
	failures := f.integer(t, `SELECT sum(attempts) FROM media_user_auth_limits`)
	login, err := store.Login(f.ctx, "synthetic.member", "Original-密码123", info)
	if err != nil {
		t.Fatal(err)
	}
	if len(login.AccessToken) != 64 || len(login.User.ID) != 32 || strings.Contains(login.User.ID, "-") {
		t.Fatal("invalid wire identity/token length")
	}
	if _, err := hex.DecodeString(login.AccessToken); err != nil {
		t.Fatal("token is not 256-bit hex")
	}
	digest := sha256.Sum256([]byte(login.AccessToken))
	if digest != login.Token.Digest || f.integer(t, `SELECT count(*) FROM media_user_tokens WHERE token_digest=$1 AND octet_length(token_digest)=32`, digest[:]) != 1 {
		t.Fatal("plaintext token was not digested")
	}
	if f.integer(t, `SELECT sum(attempts) FROM media_user_auth_limits`) != failures {
		t.Fatal("successful login counted as a failure")
	}
	if d := time.Until(login.Token.ExpiresAt); d < 364*24*time.Hour || d > 366*24*time.Hour {
		t.Fatal("unexpected token lifetime")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_devices WHERE user_id=$1 AND device_key=$2 AND name=$3 AND client=$4 AND app_version=$5 AND last_user_agent=$6 AND last_ip=$7::inet`, adminUser.ID, info.DeviceID, info.Name, info.Client, info.Version, info.UserAgent, info.ClientIP) != 1 {
		t.Fatal("device evidence missing")
	}
	u, d, tok, err := store.Authenticate(f.ctx, login.AccessToken, contracts.DeviceInfo{})
	if err != nil || u.ID != login.User.ID || d.DeviceKey != info.DeviceID || tok.Digest != digest {
		t.Fatal("token lookup failed")
	}
	f.exec(t, `UPDATE media_user_tokens SET last_seen_at=now()-interval '11 minutes',expires_at=now()+interval '1 day' WHERE token_digest=$1`, digest[:])
	if _, _, _, err := store.Authenticate(f.ctx, login.AccessToken, info); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM media_user_tokens WHERE token_digest=$1 AND expires_at>now()+interval '364 days'`, digest[:]) != 1 {
		t.Fatal("sliding extension missing")
	}
	seen := f.text(t, `SELECT last_seen_at::text FROM media_user_tokens WHERE token_digest=$1`, digest[:])
	if _, _, _, err := store.Authenticate(f.ctx, login.AccessToken, info); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT last_seen_at::text FROM media_user_tokens WHERE token_digest=$1`, digest[:]) != seen {
		t.Fatal("sliding writes were not throttled")
	}
	secondInfo := info
	secondInfo.DeviceID = "senplayer-device"
	secondInfo.Client = "SenPlayer"
	second, err := store.Login(f.ctx, "synthetic.member", "Original-密码123", secondInfo)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := store.Login(f.ctx, "synthetic.member", "Original-密码123", info)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Device.ID != login.Device.ID {
		t.Fatal("same device created another device row")
	}
	if _, _, _, err := store.Authenticate(f.ctx, login.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("old same-device token survived")
	}
	if _, _, _, err := store.Authenticate(f.ctx, second.AccessToken, secondInfo); err != nil {
		t.Fatal("other device was revoked on login")
	}
	currentUser, _, currentToken, err := store.Authenticate(f.ctx, replacement.AccessToken, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ChangePassword(f.ctx, currentUser, currentToken, "Original-密码123", "Updated-密码456"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.Authenticate(f.ctx, replacement.AccessToken, info); err != nil {
		t.Fatal("current token was lost after changing password")
	}
	if _, _, _, err := store.Authenticate(f.ctx, second.AccessToken, secondInfo); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("other device token survived password change")
	}
	f.exec(t, `UPDATE media_users SET enabled=false WHERE id=$1`, adminUser.ID)
	if _, _, _, err := store.Authenticate(f.ctx, replacement.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("disabled user accepted within cache window")
	}
	f.exec(t, `UPDATE media_users SET enabled=true,expires_at=now()-interval '1 minute' WHERE id=$1`, adminUser.ID)
	if _, _, _, err := store.Authenticate(f.ctx, replacement.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("expired user accepted")
	}
	f.exec(t, `UPDATE media_users SET expires_at=NULL WHERE id=$1`, adminUser.ID)
	_, _, currentToken, err = store.Authenticate(f.ctx, replacement.AccessToken, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Logout(f.ctx, currentToken); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.Authenticate(f.ctx, replacement.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("logout was not immediate")
	}
	fresh, err := store.Login(f.ctx, "synthetic.member", "Updated-密码456", info)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE media_users SET credential_version=credential_version+1 WHERE id=$1`, adminUser.ID)
	if _, _, _, err := store.Authenticate(f.ctx, fresh.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("credential revision change was not immediate")
	}
	revision := f.integer(t, `SELECT credential_version FROM media_users WHERE id=$1`, adminUser.ID)
	if err := (mediausers.Repository{DB: store.DB}).Mutate(f.ctx, adminUser.ID, revision, "password", "Reset-密码789"); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM media_users WHERE id=$1 AND NOT must_change_password`, adminUser.ID) != 1 {
		t.Fatal("reset should default to final password")
	}
	resetLogin, err := store.Login(f.ctx, "synthetic.member", "Reset-密码789", info)
	if err != nil {
		t.Fatal(err)
	}
	revision = f.integer(t, `SELECT credential_version FROM media_users WHERE id=$1`, adminUser.ID)
	if err := (mediausers.Repository{DB: store.DB}).Mutate(f.ctx, adminUser.ID, revision, "revoke", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.Authenticate(f.ctx, resetLogin.AccessToken, info); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("administrator revoke was not immediate")
	}
}
func TestNativeAuthFailureLimitsAndRequireChange(t *testing.T) {
	f, store, u := authFixture(t)
	info := contracts.DeviceInfo{DeviceID: "test-device", ClientIP: "93.184.216.34"}
	f.exec(t, `UPDATE media_users SET must_change_password=true WHERE id=$1`, u.ID)
	if _, err := store.Login(f.ctx, "synthetic.member", "Original-密码123", info); !errors.Is(err, contracts.ErrPasswordChangeRequired) {
		t.Fatal("password flag was ignored")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_tokens`) != 0 {
		t.Fatal("token issued while password change required")
	}
	f.exec(t, `UPDATE media_users SET must_change_password=false WHERE id=$1`, u.ID)
	for i := 0; i < 10; i++ {
		if _, err := store.Login(f.ctx, "synthetic.member", "wrong", info); !errors.Is(err, contracts.ErrUnauthenticated) {
			t.Fatal("unexpected pre-limit response")
		}
	}
	if _, err := store.Login(f.ctx, "synthetic.member", "Original-密码123", info); !errors.Is(err, contracts.ErrRateLimited) {
		t.Fatal("failure lockout missing")
	}
}

type nativeAuthSettings func(context.Context) (nativelibrary.ServerSettings, error)

func (f nativeAuthSettings) Get(ctx context.Context) (nativelibrary.ServerSettings, error) {
	return f(ctx)
}
func TestNativeEmbyRealAuthProtocolAndNoTokenInLogs(t *testing.T) {
	f, store, _ := authFixture(t)
	var logs bytes.Buffer
	value := nativelibrary.ServerSettings{Enabled: true, ServerID: strings.Repeat("a", 32), ServerName: "Synthetic Auth", Version: "4.8.10.0", PublicUserList: true}
	server, err := embyapi.NewServer(embyapi.ServerOptions{Settings: nativeAuthSettings(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }), Auth: store, Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, authorization string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "93.184.216.34:32100"
		r.Header.Set("User-Agent", "SyntheticPlayer/1")
		if authorization != "" {
			r.Header.Set("X-Emby-Authorization", authorization)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	header := `MediaBrowser Client="Infuse", Device="Synthetic iPad", DeviceId="infuse-001", Version="8.0"`
	w := call("POST", "/emby/Users/AuthenticateByName", `{"Username":"synthetic.member","Pw":"Original-密码123"}`, header)
	if w.Code != 200 {
		t.Fatalf("login status %d", w.Code)
	}
	var login dto.AuthenticationResult
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if len(login.AccessToken) != 64 || login.User.Policy.IsAdministrator || login.User.Id == "" || login.SessionInfo == nil {
		t.Fatal("invalid authentication result")
	}
	authHeader := header + `, Token="` + login.AccessToken + `"`
	w = call("GET", "/Users/"+login.User.Id, "", authHeader)
	if w.Code != 200 || strings.Contains(w.Body.String(), login.AccessToken) {
		t.Fatal("user response was unsafe")
	}
	for _, path := range []string{"/Users/" + strings.Repeat("b", 32), "/Users/" + strings.Repeat("b", 32) + "/Items", "/Users/" + strings.Repeat("b", 32) + "/Password"} {
		method := "GET"
		if strings.HasSuffix(path, "/Password") {
			method = "POST"
		}
		if got := call(method, path, `{}`, authHeader).Code; got != 403 {
			t.Fatal("cross-user access", got)
		}
	}
	public := call("GET", "/Users/Public", "", "")
	if public.Code != 200 || strings.Contains(public.Body.String(), "Policy") || strings.Contains(public.Body.String(), "password_hash") {
		t.Fatal("public user fields not minimal")
	}
	value.PublicUserList = false
	if got := call("GET", "/Users/Public", "", "").Body.String(); strings.TrimSpace(got) != "[]" {
		t.Fatal("public list disabled")
	}
	changed := call("POST", "/Users/"+login.User.Id+"/Password", `{"CurrentPw":"Original-密码123","NewPw":"NewFinal-密码123","futureField":true}`, authHeader)
	if changed.Code != 204 {
		t.Fatal("self password change", changed.Code)
	}
	if call("GET", "/Users/"+login.User.Id, "", authHeader).Code != 200 {
		t.Fatal("current token not retained")
	}
	if call("POST", "/Sessions/Logout", `{}`, authHeader).Code != 204 {
		t.Fatal("logout failed")
	}
	if call("GET", "/Users/"+login.User.Id, "", authHeader).Code != 401 {
		t.Fatal("logged out token accepted")
	}
	w = call("POST", "/Users/AuthenticateByName", `{"username":"synthetic.member","password":"NewFinal-密码123"}`, `Emby Client="SenPlayer", Device="Synthetic TV", DeviceId="sen-001", Version="6.0"`)
	if w.Code != 200 {
		t.Fatal("SenPlayer body/header aliases rejected", w.Code)
	}
	if strings.Contains(logs.String(), login.AccessToken) || strings.Contains(logs.String(), "Original-密码123") || strings.Contains(logs.String(), "NewFinal-密码123") {
		t.Fatal("plaintext credential in logs")
	}
	if !strings.Contains(logs.String(), "tokenDigestPrefix=") {
		t.Fatal("digest-only evidence absent")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_devices`) != 2 {
		t.Fatal("devices were not retained")
	}
}
