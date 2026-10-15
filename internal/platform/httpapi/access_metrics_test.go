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
	c.EmbyDirectPlayOnly = true
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
	c.EmbyDirectPlayOnly = true
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
