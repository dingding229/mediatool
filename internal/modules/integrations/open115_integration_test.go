package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type officialTransport func(*http.Request) (*http.Response, error)

func (f officialTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func officialReply(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func officialFixture(t *testing.T) (context.Context, *Manager, Record) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var databaseName string
	if err := db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil || databaseName != "media_tool_test" {
		t.Fatal("official integration tests require isolated media_tool_test")
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := Repository{DB: db, Vault: vault}
	manager := NewManager(repo, config.Config{})
	t.Cleanup(manager.Close)
	record, err := repo.Save(ctx, "", Input{Kind: "open115", Name: "official fixture", Settings: map[string]any{"appId": "12345", "cloudMountPrefix": "/115"}, Secrets: map[string]string{"accessToken": "old-access", "refreshToken": "old-refresh"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, record.ID) })
	return ctx, manager, record
}

func TestOfficialConcurrentRefreshAtomicRotationAndNoBearerDTOLeak(t *testing.T) {
	ctx, m, record := officialFixture(t)
	c, _ := m.Repository.Get(ctx, record.ID)
	c.Values["accessExpiresAt"] = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	encrypted, _ := m.Repository.Vault.Seal(record.ID, c.Values)
	m.Repository.DB.Exec(ctx, `UPDATE integrations SET credentials_ciphertext=$2 WHERE id=$1`, record.ID, encrypted)
	var calls atomic.Int32
	transport := officialTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != open115.AuthOrigin+"/open/refreshToken" {
			t.Error("wrong refresh origin")
		}
		r.ParseForm()
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("reused or wrong refresh token")
		}
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		return officialReply(r, `{"state":1,"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":7200}}`), nil
	})
	m.OutboundHTTP = &http.Client{Transport: transport}
	other := NewManager(m.Repository, m.Config)
	defer other.Close()
	other.OutboundHTTP = m.OutboundHTTP
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			manager := m
			if i%2 == 1 {
				manager = other
			}
			token, err := manager.officialToken(ctx, record.ID, "old-access", false)
			if err != nil || token != "new-access" {
				t.Errorf("refresh failed %v", err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("concurrent refresh violated 60s rule", calls.Load())
	}
	saved, _ := m.Repository.Get(ctx, record.ID)
	if saved.Values["accessToken"] != "new-access" || saved.Values["refreshToken"] != "new-refresh" {
		t.Fatal("rotated pair not atomic")
	}
	raw, _ := json.Marshal(saved.Record)
	if strings.Contains(string(raw), "new-refresh") || strings.Contains(string(raw), "new-access") {
		t.Fatal("public DTO contains credentials")
	}
}
func TestOfficialTerminalRefreshPersistsStopAndNoRepeatedRequests(t *testing.T) {
	ctx, m, record := officialFixture(t)
	var calls int
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return officialReply(r, `{"state":0,"code":40140120,"message":"PRIVATE"}`), nil
	})}
	if _, err := m.officialToken(ctx, record.ID, "old-access", false); err == nil {
		t.Fatal("invalid refresh accepted")
	}
	if _, err := m.officialToken(ctx, record.ID, "old-access", false); !errors.Is(err, open115.ErrAuthorization) {
		t.Fatal("terminal state not persisted", err)
	}
	if calls != 1 {
		t.Fatal("terminal refresh retried")
	}
	saved, _ := m.Repository.Get(ctx, record.ID)
	if saved.Values["refreshBlocked"] != "40140120" {
		t.Fatal("terminal code lost")
	}
}
func TestOfficialPathMappingAndCallerUAWithoutCD2Fallback(t *testing.T) {
	ctx, m, record := officialFixture(t)
	c, _ := m.Repository.Get(ctx, record.ID)
	c.Record.Enabled = true
	_, err := m.Repository.Save(ctx, record.ID, Input{Kind: record.Kind, Name: record.Name, Enabled: true, Settings: record.Settings, Revision: record.Revision})
	if err != nil {
		t.Fatal(err)
	}
	c, _ = m.Repository.Get(ctx, record.ID)
	var calls int
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer old-access" {
			t.Error("credential missing")
		}
		if r.ParseMultipartForm(8192) != nil {
			t.Error("official request not multipart")
		}
		if r.URL.Path == "/open/folder/get_info" {
			if r.FormValue("path") != "/Mediatool/film.mkv" {
				t.Error("virtual mount prefix not removed")
			}
			return officialReply(r, `{"state":true,"code":0,"data":{"file_id":"101","file_category":"1","file_name":"film.mkv","size_byte":4096,"pick_code":"abcdefgh12345678"}}`), nil
		}
		if r.URL.Path != "/open/ufile/downurl" || r.UserAgent() != "SenPlayer/6.2.1" {
			t.Error("caller UA not used to obtain link")
		}
		return officialReply(r, `{"state":true,"code":0,"data":{"101":{"file_name":"film.mkv","file_size":4096,"pick_code":"abcdefgh12345678","url":{"url":"https://cdnfhnfile.115cdn.net/file?SIGNED=private"}}}}`), nil
	})}
	result, err := (DynamicStorage{Manager: m}).Resolve(ctx, storage.FileRef{Path: "/115/Mediatool/film.mkv", UserAgent: "SenPlayer/6.2.1", ExpectedSize: 4096})
	if err != nil || !result.DirectOnly || !result.CallerUserAgent || result.UserAgent != "SenPlayer/6.2.1" || calls != 2 {
		t.Fatal("official route not selected", err)
	}
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		return officialReply(r, `{"state":false,"code":403,"message":"PRIVATE"}`), nil
	})}
	if _, err := (DynamicStorage{Manager: m}).Resolve(ctx, storage.FileRef{Path: "/115/Mediatool/film.mkv"}); err == nil {
		t.Fatal("official failure silently fell back to CD2")
	}
	if _, err = m.resolveOfficial(ctx, c, storage.FileRef{Path: "/other/private.mkv"}); !errors.Is(err, storage.ErrForbidden) {
		t.Fatal("wrong mount accepted")
	}
}
func TestOfficialDeviceSessionBindsAccountAndAtomicallySavesTokens(t *testing.T) {
	ctx, m, record := officialFixture(t)
	owner, ownerVersion := officialOwnerFixture(t, m)
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/open/authDeviceCode":
			return officialReply(r, `{"state":1,"code":0,"data":{"uid":"device","time":1,"sign":"SECRET-SIGN","qrcode":"https://115.com/scan/device"}}`), nil
		case "/get/status/":
			return officialReply(r, `{"state":1,"code":0,"data":{"status":2}}`), nil
		case "/open/deviceCodeToToken":
			return officialReply(r, `{"state":1,"code":0,"data":{"access_token":"qr-access","refresh_token":"qr-refresh","expires_in":7200}}`), nil
		}
		t.Fatal("unexpected auth endpoint")
		return nil, nil
	})}
	started, err := m.StartOfficialAuthorization(ctx, record.ID, owner, ownerVersion)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(started)
	if strings.Contains(string(raw), "SECRET-SIGN") || strings.Contains(string(raw), "verifier") {
		t.Fatal("verifier/sign exposed")
	}
	if _, err = m.PollOfficialAuthorization(ctx, record.ID, started.SessionID, jobs.UUID(), ownerVersion, false); !errors.Is(err, ErrAuthSession) {
		t.Fatal("other account could use session")
	}
	done, err := m.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, ownerVersion, false)
	if err != nil || done.State != "authorized" {
		t.Fatal("auth not completed", err)
	}
	saved, _ := m.Repository.Get(ctx, record.ID)
	if saved.Values["accessToken"] != "qr-access" || saved.Values["refreshToken"] != "qr-refresh" || saved.Record.Enabled {
		t.Fatal("token pair not saved or provider enabled without request")
	}
	var pending []byte
	m.Repository.DB.QueryRow(ctx, `SELECT payload_ciphertext FROM open115_authorizations WHERE id=$1`, started.SessionID).Scan(&pending)
	if len(pending) != 0 {
		t.Fatal("completed verifier retained")
	}
}

// Uses the existing isolated-test administrator if present; never touches a
// business database. The caller's fixture connection is cleaned up separately.
func officialOwnerFixture(t *testing.T, m *Manager) (string, int64) {
	t.Helper()
	ctx := context.Background()
	owner := jobs.UUID()
	var existing string
	err := m.Repository.DB.QueryRow(ctx, `SELECT id::text FROM administrator_accounts LIMIT 1`).Scan(&existing)
	if err == nil {
		owner = existing
	} else {
		_, err = m.Repository.DB.Exec(ctx, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES($1,'official_fixture','fixture')`, owner)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { m.Repository.DB.Exec(ctx, `DELETE FROM administrator_accounts WHERE id=$1`, owner) })
	}
	var ownerVersion int64
	m.Repository.DB.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1`, owner).Scan(&ownerVersion)
	return owner, ownerVersion
}

type officialAuthOutcome struct {
	result OfficialAuthorization
	err    error
}

func awaitOfficialAuth(t *testing.T, results <-chan officialAuthOutcome) officialAuthOutcome {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("authorization operation did not finish")
		return officialAuthOutcome{}
	}
}

func TestOfficialLongPollDoesNotBlockPlaybackOrCancellation(t *testing.T) {
	ctx, m, record := officialFixture(t)
	owner, version := officialOwnerFixture(t, m)
	entered, release := make(chan struct{}), make(chan struct{})
	var exchanges atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/open/authDeviceCode":
			return officialReply(r, `{"state":1,"data":{"uid":"device","time":1,"sign":"SIGN","qrcode":"https://115.com/scan/device"}}`), nil
		case "/get/status/":
			close(entered)
			<-release
			return officialReply(r, `{"state":1,"data":{"status":2}}`), nil
		case "/open/deviceCodeToToken":
			exchanges.Add(1)
			return officialReply(r, `{"state":1,"data":{"access_token":"new","refresh_token":"fresh","expires_in":7200}}`), nil
		}
		return nil, errors.New("unexpected fixture endpoint")
	})}
	started, err := m.StartOfficialAuthorization(ctx, record.ID, owner, version)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan officialAuthOutcome, 1)
	go func() {
		result, err := m.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
		results <- officialAuthOutcome{result, err}
	}()
	defer close(release)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("long poll did not start")
	}
	short, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	if token, err := m.officialToken(short, record.ID, "", false); err != nil || token != "old-access" {
		t.Fatalf("QR poll blocked playback token: %v", err)
	}
	cancelled, err := m.PollOfficialAuthorization(short, record.ID, started.SessionID, owner, version, true)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("QR poll blocked cancellation: %v", err)
	}
	// The deferred close unblocks the provider; inspect the result in cleanup,
	// before the fixture database/manager are closed.
	t.Cleanup(func() {
		done := awaitOfficialAuth(t, results)
		if done.err != nil || done.result.State != "cancelled" || exchanges.Load() != 0 {
			t.Fatalf("stale poll exchanged a cancelled code: %v", done.err)
		}
	})
}

func TestOfficialExchangeClaimPersistsAndSerializesAcrossManagers(t *testing.T) {
	ctx, m, record := officialFixture(t)
	owner, version := officialOwnerFixture(t, m)
	entered, release := make(chan struct{}), make(chan struct{})
	var exchanges atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/open/authDeviceCode":
			return officialReply(r, `{"state":1,"data":{"uid":"device","time":1,"sign":"SIGN","qrcode":"https://115.com/scan/device"}}`), nil
		case "/get/status/":
			return officialReply(r, `{"state":1,"data":{"status":2}}`), nil
		case "/open/deviceCodeToToken":
			if exchanges.Add(1) != 1 {
				t.Error("device code exchanged more than once")
			}
			close(entered)
			<-release
			return officialReply(r, `{"state":1,"data":{"access_token":"qr-access","refresh_token":"qr-refresh","expires_in":7200}}`), nil
		}
		return nil, errors.New("unexpected fixture endpoint")
	})}
	started, err := m.StartOfficialAuthorization(ctx, record.ID, owner, version)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan officialAuthOutcome, 1)
	go func() {
		result, err := m.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
		results <- officialAuthOutcome{result, err}
	}()
	defer close(release)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("exchange did not start")
	}
	var state string
	var payload []byte
	if err := m.Repository.DB.QueryRow(ctx, `SELECT state,payload_ciphertext FROM open115_authorizations WHERE id=$1`, started.SessionID).Scan(&state, &payload); err != nil || state != "exchanging" || len(payload) != 0 {
		t.Fatal("exchange claim/verifier erasure not committed before request", err)
	}
	other := NewManager(m.Repository, m.Config)
	t.Cleanup(other.Close)
	other.OutboundHTTP = m.OutboundHTTP
	short, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	pending, err := other.PollOfficialAuthorization(short, record.ID, started.SessionID, owner, version, false)
	if err != nil || pending.State != "exchanging" {
		t.Fatal("second process replayed/waited on exchange", err)
	}
	if _, err := other.officialToken(short, record.ID, "", false); err != nil {
		t.Fatal("exchange held playback lock", err)
	}
	t.Cleanup(func() {
		done := awaitOfficialAuth(t, results)
		if done.err != nil || done.result.State != "authorized" || exchanges.Load() != 1 {
			t.Fatal("exchange did not finish once", done.err)
		}
	})
}

func TestOfficialLostExchangeResponseCannotReplayAfterRestart(t *testing.T) {
	ctx, m, record := officialFixture(t)
	owner, version := officialOwnerFixture(t, m)
	requestCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	var exchanges atomic.Int32
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/open/authDeviceCode":
			return officialReply(r, `{"state":1,"data":{"uid":"device","time":1,"sign":"SIGN","qrcode":"https://115.com/scan/device"}}`), nil
		case "/get/status/":
			return officialReply(r, `{"state":1,"data":{"status":2}}`), nil
		case "/open/deviceCodeToToken":
			exchanges.Add(1)
			disconnect() // upstream might have consumed the code; browser gone
			return nil, context.Canceled
		}
		return nil, errors.New("unexpected fixture endpoint")
	})}
	started, err := m.StartOfficialAuthorization(ctx, record.ID, owner, version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.PollOfficialAuthorization(requestCtx, record.ID, started.SessionID, owner, version, false); err == nil {
		t.Fatal("lost response accepted")
	}
	other := NewManager(m.Repository, m.Config)
	defer other.Close()
	other.OutboundHTTP = m.OutboundHTTP
	failed, err := other.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
	if err != nil || failed.State != "failed" || exchanges.Load() != 1 {
		t.Fatal("lost response replayed", err)
	}
	// Simulate a process dying AFTER its durable claim and BEFORE saving outcome.
	if _, err = m.Repository.DB.Exec(ctx, `UPDATE open115_authorizations SET state='exchanging' WHERE id=$1`, started.SessionID); err != nil {
		t.Fatal(err)
	}
	pending, err := other.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
	if err != nil || pending.State != "exchanging" || exchanges.Load() != 1 {
		t.Fatal("orphaned claim replayed", err)
	}
	m.Repository.DB.Exec(ctx, `UPDATE open115_authorizations SET expires_at=now()-interval '1 second' WHERE id=$1`, started.SessionID)
	expired, err := other.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
	if err != nil || expired.State != "expired" || exchanges.Load() != 1 {
		t.Fatal("orphaned claim did not expire safely", err)
	}
	saved, _ := m.Repository.Get(ctx, record.ID)
	if saved.Values["accessToken"] != "old-access" || saved.Values["refreshToken"] != "old-refresh" {
		t.Fatal("failed exchange overwrote existing tokens")
	}
}

func TestOfficialStalePollOrExchangeCannotOverwriteNewerState(t *testing.T) {
	for _, change := range []string{"cancel_poll", "edit_poll", "owner_poll", "cancel_exchange", "replace_exchange", "edit_exchange", "owner_exchange"} {
		t.Run(change, func(t *testing.T) {
			ctx, m, record := officialFixture(t)
			owner, version := officialOwnerFixture(t, m)
			entered, release := make(chan struct{}), make(chan struct{})
			var exchanges atomic.Int32
			blockExchange := strings.HasSuffix(change, "_exchange")
			m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/open/authDeviceCode":
					return officialReply(r, `{"state":1,"data":{"uid":"device","time":1,"sign":"SIGN","qrcode":"https://115.com/scan/device"}}`), nil
				case "/get/status/":
					if !blockExchange {
						close(entered)
						<-release
					}
					return officialReply(r, `{"state":1,"data":{"status":2}}`), nil
				case "/open/deviceCodeToToken":
					exchanges.Add(1)
					if blockExchange {
						close(entered)
						<-release
					}
					return officialReply(r, `{"state":1,"data":{"access_token":"stale-access","refresh_token":"stale-refresh","expires_in":7200}}`), nil
				}
				return nil, errors.New("unexpected fixture endpoint")
			})}
			started, err := m.StartOfficialAuthorization(ctx, record.ID, owner, version)
			if err != nil {
				t.Fatal(err)
			}
			results := make(chan officialAuthOutcome, 1)
			go func() {
				result, err := m.PollOfficialAuthorization(ctx, record.ID, started.SessionID, owner, version, false)
				results <- officialAuthOutcome{result, err}
			}()
			defer close(release)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("provider request did not start")
			}
			expected := "expired"
			short, stop := context.WithTimeout(ctx, 2*time.Second)
			defer stop()
			switch {
			case strings.HasPrefix(change, "cancel"):
				expected = "cancelled"
				result, err := m.PollOfficialAuthorization(short, record.ID, started.SessionID, owner, version, true)
				if err != nil || result.State != expected {
					t.Fatal("explicit cancel blocked", err)
				}
			case strings.HasPrefix(change, "replace"):
				expected = "cancelled"
				replacement, err := m.StartOfficialAuthorization(short, record.ID, owner, version)
				if err != nil || replacement.SessionID == started.SessionID {
					t.Fatal("replacement blocked", err)
				}
			case strings.HasPrefix(change, "edit"):
				_, err := m.Repository.Save(short, record.ID, Input{Kind: record.Kind, Name: "edited", Revision: record.Revision, Settings: record.Settings})
				if err != nil {
					t.Fatal("edit blocked", err)
				}
			case strings.HasPrefix(change, "owner"):
				_, err := m.Repository.DB.Exec(short, `UPDATE administrator_accounts SET credential_version=credential_version+1 WHERE id=$1`, owner)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					m.Repository.DB.Exec(ctx, `UPDATE administrator_accounts SET credential_version=$2 WHERE id=$1`, owner, version)
				})
			}
			t.Cleanup(func() {
				done := awaitOfficialAuth(t, results)
				if done.err != nil || done.result.State != expected {
					t.Fatal("stale outcome changed state", done.err)
				}
				if !blockExchange && exchanges.Load() != 0 {
					t.Fatal("stale poll exchanged code")
				}
				saved, err := m.Repository.Get(ctx, record.ID)
				if err != nil || saved.Values["accessToken"] != "old-access" || saved.Values["refreshToken"] != "old-refresh" {
					t.Fatal("stale outcome overwrote credentials", err)
				}
			})
		})
	}
}

func TestOfficialResolutionPhaseEvidenceKeepsUAAndProviderFailuresSeparate(t *testing.T) {
	ctx, m, record := officialFixture(t)
	if _, err := m.Repository.Save(ctx, record.ID, Input{Kind: record.Kind, Name: record.Name, Enabled: true, Settings: record.Settings, Revision: record.Revision}); err != nil {
		t.Fatal(err)
	}
	var events []storage.ResolveObservation
	observed := storage.WithResolveObserver(ctx, func(e storage.ResolveObservation) { events = append(events, e) })
	m.OutboundHTTP = &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/open/folder/get_info" {
			return officialReply(r, `{"state":true,"data":{"file_id":"101","file_category":"1","file_name":"film.mkv","size_byte":4096,"pick_code":"abcdefgh12345678"}}`), nil
		}
		if r.URL.Path == "/open/ufile/downurl" && r.UserAgent() == "PRIVATE-ACTUAL-UA" {
			response := officialReply(r, `{"state":false,"code":40140117,"message":"PRIVATE-TOKEN"}`)
			response.StatusCode = 429
			return response, nil
		}
		return nil, errors.New("PRIVATE-FORBIDDEN-FALLBACK")
	})}
	_, err := (DynamicStorage{Manager: m}).Resolve(observed, storage.FileRef{Path: "/115/Mediatool/film.mkv", UserAgent: "PRIVATE-ACTUAL-UA", ExpectedSize: 4096})
	var api *open115.APIError
	if !errors.As(err, &api) || !api.RateLimited() {
		t.Fatal("official failure lost classification", err)
	}
	phases := map[storage.ResolveStage]storage.ResolveObservation{}
	for _, e := range events {
		phases[e.Stage] = e
	}
	for _, phase := range []storage.ResolveStage{storage.StageCloudProfile, storage.StageOfficialToken, storage.StageOfficialHTTPPath, storage.StageOfficialPath, storage.StageOfficialHTTPDownload, storage.StageOfficialDownload} {
		if _, ok := phases[phase]; !ok {
			t.Fatal("missing phase", phase)
		}
	}
	if phases[storage.StageOfficialPath].Outcome != "ok" || phases[storage.StageOfficialDownload].Outcome != "rate_limited" || phases[storage.StageOfficialDownload].ProviderCode != 40140117 {
		t.Fatal("failed stage not identifiable")
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("raw provider/client data in observation")
	}
}
