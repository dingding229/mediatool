package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"encoding/json"
	"io"
	"log/slog"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessMetricsRawURLAndLocationWithoutCollectingHeadersOrBodies(t *testing.T) {
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	handler := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(206)
		io.WriteString(w, "owned-bytes")
	}))
	request := httptest.NewRequest("GET", "/emby/videos/11/hls1/main/0.ts?api_key=NEVER-LOG-TOKEN&sign=NEVER-LOG-SIGN", nil)
	request.Header.Set("Authorization", "Bearer NEVER-LOG-AUTH")
	request.AddCookie(&http.Cookie{Name: "x", Value: "NEVER-LOG-COOKIE"})
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, request)
	var metric map[string]any
	if json.Unmarshal(logs.Bytes(), &metric) != nil {
		t.Fatal("invalid access metric")
	}
	if metric["status"] != float64(206) || metric["responseBytes"] != float64(11) || metric["path"] != "/emby/videos/11/hls1/main/0.ts" {
		t.Fatal("missing HTTP facts", metric)
	}
	if !strings.Contains(logs.String(), "NEVER-LOG-TOKEN") || !strings.Contains(logs.String(), "NEVER-LOG-SIGN") || strings.Contains(logs.String(), "NEVER-LOG-AUTH") || strings.Contains(logs.String(), "NEVER-LOG-COOKIE") || strings.Contains(logs.String(), "owned-bytes") {
		t.Fatal("raw URL missing or unrequested header/body collected")
	}
}
func TestAccessMetricsPreserveWriterInterfacesAndFlush(t *testing.T) {
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	writer := httptest.NewRecorder()
	handler := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("stream flushing interface hidden")
		}
		if _, ok := w.(http.Hijacker); ok {
			t.Fatal("unsupported hijacking interface falsely added")
		}
		if _, ok := w.(io.ReaderFrom); ok {
			t.Fatal("unsupported ReaderFrom falsely added")
		}
		io.WriteString(w, "event: ready\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Fatal(err)
		}
	}))
	handler.ServeHTTP(writer, httptest.NewRequest("GET", "/api/v1/events", nil))
	if !writer.Flushed {
		t.Fatal("stream flush did not reach underlying writer")
	}
	var metric map[string]any
	json.Unmarshal(logs.Bytes(), &metric)
	if metric["status"] != float64(200) || metric["responseBytes"] != float64(14) {
		t.Fatal("implicit write status lost", metric)
	}
}
func TestAccessMetricsIncludeRecoveredPanicWithoutPayload(t *testing.T) {
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	writer := httptest.NewRecorder()
	handler := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("NEVER-LOG-PRIVATE-PANIC") }))
	handler.ServeHTTP(writer, httptest.NewRequest("GET", "/api/v1/fixture", nil))
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	var metric map[string]any
	json.Unmarshal([]byte(lines[len(lines)-1]), &metric)
	if writer.Code != 500 || metric["status"] != float64(500) || metric["responseBytes"].(float64) <= 0 {
		t.Fatal("panic response not captured")
	}
	if strings.Contains(logs.String(), "NEVER-LOG") {
		t.Fatal("panic payload leaked")
	}
}

func TestAccessMetricsDoNotAppendJSONToAbortedMediaStream(t *testing.T) {
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	writer := httptest.NewRecorder()
	handler := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(206)
		io.WriteString(w, "owned-mpegts-prefix")
		panic(http.ErrAbortHandler)
	}))
	func() {
		defer func() {
			if recovered := recover(); recovered != http.ErrAbortHandler {
				t.Fatal("stream abort sentinel swallowed", recovered)
			}
		}()
		handler.ServeHTTP(writer, httptest.NewRequest("GET", "/emby/videos/11/hls1/main/0.ts?api_key=NEVER-LOG-ABORT-TOKEN", nil))
	}()
	if writer.Body.String() != "owned-mpegts-prefix" || writer.Code != 206 {
		t.Fatal("JSON appended to truncated binary stream", writer.Body.String())
	}
	if !strings.Contains(logs.String(), "HTTP stream aborted") || strings.Contains(logs.String(), "NEVER-LOG") || strings.Contains(logs.String(), "owned-mpegts-prefix") {
		t.Fatal("unsafe abort log")
	}
}

func TestCancelledStreamLogsFixedReasonWithoutFabricated502(t *testing.T) {
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	handler := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Media-Gateway-Reason", "request_cancelled")
		panic(http.ErrAbortHandler)
	}))
	func() {
		defer func() {
			if recovered := recover(); recovered != http.ErrAbortHandler {
				t.Fatal("cancelled request swallowed")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/emby/videos/11/hls1/main/0.ts?api_key=NEVER-LOG", nil))
	}()
	// A random hexadecimal requestId can contain "502". Inspect the status
	// field rather than making cancellation verification probabilistic.
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal("invalid cancellation log", err)
	}
	if record["msg"] != "HTTP stream aborted" || record["X-Media-Gateway-Reason"] != "request_cancelled" || record["status"] == float64(502) || strings.Contains(logs.String(), "NEVER-LOG") {
		t.Fatal("misleading/unsafe cancellation log")
	}
}

func TestResolveStageAccessLogsAreRequestCorrelatedAndNeverContainSecrets(t *testing.T) {
	var logs bytes.Buffer
	c := cfg()
	c.PlaybackResolveTimeout = 25 * time.Millisecond
	source := resolverFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		storage.ObserveResolve(ctx, storage.StageOfficialPath, time.Now(), errors.New("PRIVATE-TOKEN https://secret.test/?PRIVATE-SIGNATURE"))
		storage.ObserveResolve(ctx, storage.ResolveStage("PRIVATE-PATH"), time.Now(), nil)
		<-ctx.Done()
		return storage.DownloadLink{}, ctx.Err()
	})
	handler := New(c, Dependencies{Storage: source, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	r := httptest.NewRequest("GET", linkTarget(), nil)
	r.Header.Set("User-Agent", "PRIVATE-UA")
	r.Header.Set("X-Request-ID", "PRIVATE-ID")
	r.Header.Set("Authorization", "PRIVATE-BEARER")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 504 || strings.Contains(logs.String(), "PRIVATE-BEARER") || strings.Contains(logs.String(), "PRIVATE-SIGNATURE") || !strings.Contains(logs.String(), "PRIVATE-UA") || !strings.Contains(logs.String(), c.STRMKey) {
		t.Fatal("timeout or stage redaction changed", w.Code)
	}
	id := w.Header().Get("X-Request-ID")
	stages := 0
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var d map[string]any
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatal(err)
		}
		if d["requestId"] != id {
			t.Fatal("stage lost generated correlation")
		}
		if d["msg"] == "Playback resolution stage" {
			stages++
			if d["stage"] == "resolve" && d["outcome"] != "timeout" {
				t.Fatal("wrong failure classification")
			}
		}
	}
	if stages != 2 {
		t.Fatal("invalid/sensitive stage was logged", stages)
	}
}
func TestFixedSTRMDeadlineCanExceedManagementWriteTimeoutButRemainsBounded(t *testing.T) {
	c := cfg()
	c.PlaybackResolveTimeout = 200 * time.Millisecond
	source := resolverFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		select {
		case <-time.After(80 * time.Millisecond):
			return storage.DownloadLink{URL: "https://cdn.example.com/owned"}, nil
		case <-ctx.Done():
			return storage.DownloadLink{}, ctx.Err()
		}
	})
	engine := playback.New(source, playback.Options{ResolveTimeout: c.PlaybackResolveTimeout, Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	h := New(c, Dependencies{Storage: source, Playback: engine, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	server := httptest.NewUnstartedServer(h)
	server.Config.WriteTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()
	parsed, _ := url.Parse(linkTarget())
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(server.URL + parsed.RequestURI())
	if err != nil {
		t.Fatal("management deadline clipped bounded STRM response", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 302 || response.Header.Get("Location") != "https://cdn.example.com/owned" {
		t.Fatal("expected direct response", response.StatusCode)
	}
	// Assert finite bounds separately; no removal of deadline for this endpoint.
	if requestbudget.FixedSTRMWrite(c.PlaybackResolveTimeout) > 18*time.Second {
		t.Fatal("unbounded deadline")
	}
}

type traceFixtureTransport func(*http.Request) (*http.Response, error)

func (f traceFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialHTTPStageTimingFieldsReachAdministratorLogsWithoutSecrets(t *testing.T) {
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "PRIVATE-PROVIDER-COOKIE")
		switch r.URL.Path {
		case "/open/folder/get_info":
			io.WriteString(w, `{"state":true,"data":{"file_id":"1","file_name":"PRIVATE-Film.mkv","size_byte":16,"pick_code":"abcdefgh12345678","file_category":"1"}}`)
		case "/open/ufile/downurl":
			io.WriteString(w, `{"state":true,"data":{"1":{"file_name":"PRIVATE-Film.mkv","file_size":16,"pick_code":"abcdefgh12345678","url":{"url":"https://cdn.example.test/movie?PRIVATE-SIGNATURE"}}}}`)
		case "/open/refreshToken":
			io.WriteString(w, `{"state":true,"data":{"access_token":"PRIVATE-NEW-ACCESS","refresh_token":"PRIVATE-NEW-REFRESH","expires_in":7200}}`)
		default:
			t.Error("unexpected official request path")
			w.WriteHeader(500)
		}
	}))
	defer provider.Close()
	upstream, _ := url.Parse(provider.URL)
	transport := provider.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := open115.Client{HTTP: &http.Client{Timeout: time.Second, Transport: traceFixtureTransport(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.Scheme, clone.URL.Host = upstream.Scheme, upstream.Host
		return transport.RoundTrip(clone)
	})}}
	var logs bytes.Buffer
	api := &API{Config: cfg(), Deps: Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
	h := api.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, err := client.FileByPath(ctx, "PRIVATE-ACCESS-TOKEN", "/PRIVATE-Film.mkv"); err != nil {
			t.Error(err)
		}
		if _, err := client.Download(ctx, "PRIVATE-ACCESS-TOKEN", "abcdefgh12345678", "OwnedPlayer"); err != nil {
			t.Error(err)
		}
		if _, err := client.Refresh(ctx, "PRIVATE-REFRESH-TOKEN"); err != nil {
			t.Error(err)
		}
		w.WriteHeader(204)
	}))
	r := httptest.NewRequest("GET", "/diagnostic", nil)
	r.Header.Set("Authorization", "PRIVATE-CALLER-AUTH")
	r.AddCookie(&http.Cookie{Name: "fixture", Value: "PRIVATE-CALLER-COOKIE"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || strings.Contains(logs.String(), "PRIVATE") {
		t.Fatal("HTTP trace leaked credentials/path/signature/cookies", logs.String())
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] != "Playback resolution stage" {
			continue
		}
		seen[record["stage"].(string)] = true
		for _, key := range []string{"dnsMs", "tcpMs", "tlsMs", "firstByteMs", "httpTotalMs", "httpAttempt"} {
			if _, ok := record[key].(float64); !ok {
				t.Fatal("missing numeric admin log timing", key, record)
			}
		}
		if record["httpAttempt"] != float64(1) || record["httpResponseReceived"] != true || record["requestId"] != w.Header().Get("X-Request-ID") {
			t.Fatal(record)
		}
	}
	for _, stage := range []string{"official_http_path", "official_http_download", "official_http_refresh"} {
		if !seen[stage] {
			t.Fatal("missing official stage", stage)
		}
	}
}
