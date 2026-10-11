package open115

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"time"

	"media-tool/internal/modules/storage"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func TestOfficialDownloadUsesExactPlayerUAAndMultipartBearer(t *testing.T) {
	calls := 0
	c := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != APIOrigin+"/open/ufile/downurl" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private-token" || r.UserAgent() != "SenPlayer/6.2.1" {
			t.Error("official request contract changed")
		}
		if err := r.ParseMultipartForm(8192); err != nil || r.FormValue("pick_code") != "abcdefgh12345678" {
			t.Error("pick code not multipart")
		}
		return reply(r, `{"state":true,"code":0,"data":{"123":{"file_name":"film.mkv","file_size":10000,"pick_code":"abcdefgh12345678","url":{"url":"https://cdnfhnfile.115cdn.net/media?private=SIGNATURE"}}}}`), nil
	})}}
	result, err := c.Download(context.Background(), "private-token", "abcdefgh12345678", "SenPlayer/6.2.1")
	if err != nil || result.PickCode != "abcdefgh12345678" || calls != 1 {
		t.Fatal("download decode failed", err)
	}
}
func TestPKCEUsesBinarySHA256URLSafeNoPadding(t *testing.T) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(verifier))
	if len(verifier) < 43 || len(verifier) > 128 || challenge != base64.RawURLEncoding.EncodeToString(h[:]) || strings.Contains(challenge, "=") {
		t.Fatal("PKCE differs from official specification")
	}
	other, _, _ := NewPKCE()
	if other == verifier {
		t.Fatal("reused verifier")
	}
}
func TestOfficialErrorsNeverEchoProviderSecretsAndClassifyTerminalRefresh(t *testing.T) {
	for _, code := range []int64{40140116, 40140119, 40140120, 40140137} {
		c := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			raw, _ := json.Marshal(map[string]any{"state": 0, "code": code, "message": "TOKEN-SECRET https://cdn.example?secret=PRIVATE"})
			return reply(r, string(raw)), nil
		})}}
		_, err := c.Refresh(context.Background(), "REFRESH-SECRET")
		var api *APIError
		if !errors.As(err, &api) || !api.TerminalRefresh() || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "https:") {
			t.Fatal("unsafe error classification", err)
		}
	}
}
func TestFileInfoSupportsPathAndNumericOrStringIDs(t *testing.T) {
	c := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/open/folder/get_info" || r.ParseMultipartForm(8192) != nil || r.FormValue("path") != "/Mediatool/film.mkv" {
			t.Error("file mapping contract changed")
		}
		return reply(r, `{"state":true,"code":0,"data":{"file_id":12345,"file_category":1,"file_name":"film.mkv","size_byte":100,"pick_code":"abcdefgh12345678"}}`), nil
	})}}
	f, err := c.FileByPath(context.Background(), "token", "/Mediatool/film.mkv")
	if err != nil || f.ID != "12345" || f.Category != "1" {
		t.Fatal("numeric IDs not decoded", err)
	}
}
func TestCrossOriginRedirectNotFollowedAndMalformedDownloadRejected(t *testing.T) {
	calls := 0
	c := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		res := reply(r, `{}`)
		res.StatusCode = 302
		res.Header.Set("Location", "https://foreign.example/token")
		return res, nil
	})}}
	if _, err := c.Refresh(context.Background(), "PRIVATE"); err == nil || calls != 1 {
		t.Fatal("bearer redirect followed")
	}
	for _, raw := range []string{`{"state":true,"data":{}}`, `{"state":true,"data":{"id":{"pick_code":"other123456","url":{"url":"https://cdn.example/media"}}}}`} {
		c.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return reply(r, raw), nil })}
		if _, err := c.Download(context.Background(), "token", "abcdefgh12345678", "SenPlayer"); err == nil {
			t.Fatal("ambiguous/mismatched link accepted")
		}
	}
}

func TestOfficialDefaultClientsShareBoundedTransportWithoutJarOrProxy(t *testing.T) {
	first, second := (Client{}).client(), (Client{}).client()
	if first.Transport != second.Transport || first.Jar != nil || second.Jar != nil {
		t.Fatal("default connection pooling or credential isolation changed")
	}
	tr, ok := first.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil || tr.MaxConnsPerHost <= 0 || tr.MaxIdleConnsPerHost <= 0 || tr.IdleConnTimeout <= 0 || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("unbounded/unsafe official transport")
	}
	if first.CheckRedirect(nil, nil) != http.ErrUseLastResponse || first.Timeout != 12*time.Second {
		t.Fatal("redirect/per-call policy changed")
	}
}
func TestOfficialPooledRequestsReuseSocketButNeverReuseBearerOrUA(t *testing.T) {
	var connections atomic.Int32
	seen := make(chan string, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Authorization") + "|" + r.UserAgent()
		io.WriteString(w, `{"state":true,"code":0,"data":{}}`)
	})
	server := httptest.NewUnstartedServer(handler)
	server.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()
	tr := newOfficialTransport()
	tr.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	defer tr.CloseIdleConnections()
	c := Client{HTTP: &http.Client{Transport: tr, Timeout: time.Second}}
	for _, token := range []string{"token-a", "token-b"} {
		if err := c.call(context.Background(), "GET", server.URL, token, "UA-"+token, nil, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-seen, <-seen
	if connections.Load() != 1 || first != "Bearer token-a|UA-token-a" || second != "Bearer token-b|UA-token-b" {
		t.Fatal("socket not pooled or credentials mixed", connections.Load())
	}
}
func TestOfficialNetworkFailuresKeepTimeoutIdentityWithoutSecretURL(t *testing.T) {
	for _, test := range []struct {
		name     string
		parent   bool
		deadline bool
		want     error
	}{{"parent-cancelled", true, false, context.Canceled}, {"parent-deadline", false, true, context.DeadlineExceeded}, {"http-deadline", false, false, storage.ErrRequestTimeout}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.parent {
				var stop context.CancelFunc
				ctx, stop = context.WithCancel(ctx)
				stop()
			}
			if test.deadline {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			var events []storage.ResolveObservation
			ctx = storage.WithResolveObserver(ctx, func(e storage.ResolveObservation) { events = append(events, e) })
			c := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "PRIVATE-TOKEN", URL: "https://secret.test/PRIVATE-SIGNATURE", Err: context.DeadlineExceeded}
			})}}
			_, err := c.Download(ctx, "PRIVATE-TOKEN", "abcdefgh12345678", "PRIVATE-UA")
			if !errors.Is(err, test.want) || strings.Contains(err.Error(), "PRIVATE") || len(events) != 1 || events[0].Stage != storage.StageOfficialHTTPDownload {
				t.Fatal("network error lost identity/leaked", err)
			}
		})
	}
}
