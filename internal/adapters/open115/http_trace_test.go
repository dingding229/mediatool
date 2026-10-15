package open115

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
)

const fileResponse = `{"state":true,"code":0,"data":{"file_id":"1","file_name":"Film.mkv","size_byte":16,"pick_code":"abcdefgh12345678","file_category":"1"}}`

func officialFixtureClient(t *testing.T, handler http.Handler, timeout time.Duration) Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	target, _ := url.Parse(server.URL)
	return Client{HTTP: &http.Client{Timeout: timeout, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		copyURL := *r.URL
		request.URL = &copyURL
		request.URL.Scheme, request.URL.Host = target.Scheme, target.Host
		return transport.RoundTrip(request)
	})}}
}

func TestPathQueryRealHTTPTimeoutRetryAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		alwaysTimeout, shortBudget bool
		want                       int32
	}{
		{"first timeout second succeeds", false, false, 2},
		{"both attempts timeout", true, false, 2},
		{"remaining budget insufficient", true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			client := officialFixtureClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(8192); err != nil || r.FormValue("path") != "/Film.mkv" || r.Header.Get("Authorization") != "Bearer PRIVATE-TOKEN" {
					t.Error("retry did not reconstruct the original request")
				}
				if n := count.Add(1); n == 1 || tc.alwaysTimeout {
					<-r.Context().Done()
					return
				}
				io.WriteString(w, fileResponse)
			}), 100*time.Millisecond)
			budget := 2 * time.Second
			if tc.shortBudget {
				budget = 150 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			var events []storage.ResolveObservation
			ctx = storage.WithResolveObserver(ctx, func(e storage.ResolveObservation) { events = append(events, e) })
			_, err := client.FileByPath(ctx, "PRIVATE-TOKEN", "/Film.mkv")
			if count.Load() != tc.want || (err == nil) == tc.alwaysTimeout {
				t.Fatalf("calls=%d err=%v", count.Load(), err)
			}
			if len(events) != int(tc.want) {
				t.Fatal("wrong observation count", len(events))
			}
			for i, event := range events {
				if event.HTTPTrace == nil || event.HTTPTrace.Attempt != i+1 || event.HTTPTrace.TotalMillis < 0 || event.HTTPTrace.ResponseReceived != (err == nil && i == 1) {
					t.Fatal("unsafe or incomplete timing observation", event)
				}
			}
		})
	}
}

func TestPathQueryNeverRetriesHTTPResponsesOrBodyTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		stallBody bool
		interim   bool
	}{
		{"429", 429, `{"state":false,"code":40140117}`, false, false},
		{"expired token", 401, `{"state":false,"code":40140125}`, false, false},
		{"token code with HTTP 200", 200, `{"state":false,"code":40140125}`, false, false},
		{"risk control", 403, `<html>PRIVATE-SIGNATURE</html>`, false, false},
		{"upstream 500", 500, `{"state":false,"code":500}`, false, false},
		{"upstream 503", 503, `<html>busy</html>`, false, false},
		{"body timeout after headers", 200, "", true, false},
		{"100 response before final header timeout", 100, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			client := officialFixtureClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(tc.status)
				if tc.interim {
					<-r.Context().Done()
					return
				}
				if tc.stallBody {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				io.WriteString(w, tc.body)
			}), 100*time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.FileByPath(ctx, "PRIVATE-TOKEN", "/Film.mkv")
			if err == nil || count.Load() != 1 {
				t.Fatal("HTTP response was retried", count.Load(), err)
			}
		})
	}
}

func TestOnlyPathQueryRetriesConnectionResetWithoutResponse(t *testing.T) {
	for _, operation := range []string{"path", "download", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int32
			owned := officialFixtureClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fileResponse) }), 100*time.Millisecond)
			underlying := owned.HTTP.Transport
			owned.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
				}
				return underlying.RoundTrip(r)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var err error
			switch operation {
			case "path":
				_, err = owned.FileByPath(ctx, "PRIVATE-TOKEN", "/Film.mkv")
			case "download":
				_, err = owned.Download(ctx, "PRIVATE-TOKEN", "abcdefgh12345678", "Player")
			case "refresh":
				_, err = owned.Refresh(ctx, "PRIVATE-REFRESH")
			}
			if operation == "path" {
				if err != nil || calls.Load() != 2 {
					t.Fatal(calls.Load(), err)
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatal("non-path request retried", calls.Load(), err)
			}
		})
	}
}

func TestHTTPTracePhasesAreRaceSafeAndOnlyEmitNumericMetadata(t *testing.T) {
	trace := newHTTPTrace(time.Now(), 1)
	callbacks := httptrace.ContextClientTrace(trace.traceContext(context.Background()))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callbacks.DNSStart(httptrace.DNSStartInfo{Host: "PRIVATE-TOKEN"})
			callbacks.DNSDone(httptrace.DNSDoneInfo{})
			callbacks.ConnectStart("tcp", "PRIVATE-IP:443")
			callbacks.ConnectDone("tcp", "PRIVATE-IP:443", errors.New("PRIVATE-COOKIE"))
			callbacks.TLSHandshakeStart()
			callbacks.TLSHandshakeDone(tls.ConnectionState{}, errors.New("PRIVATE-SIGNATURE"))
			callbacks.GotConn(httptrace.GotConnInfo{Reused: true})
			callbacks.GotFirstResponseByte()
		}()
	}
	wg.Wait()
	result := trace.snapshot()
	if result.DNSMillis < 0 || result.TCPMillis < 0 || result.TLSMillis < 0 || result.FirstByteMillis < 0 {
		t.Fatal("phase not observed", result)
	}
	before, _ := json.Marshal(result)
	callbacks.GotFirstResponseByte()
	after, _ := json.Marshal(result)
	if string(before) != string(after) || strings.Contains(string(before), "PRIVATE") {
		t.Fatal("late mutation or sensitive observation", string(before), string(after))
	}
}

type retryPlaybackResolver struct{ client Client }

func (r retryPlaybackResolver) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	_, err := r.client.FileByPath(ctx, "PRIVATE-TOKEN", "/Film.mkv")
	if err != nil {
		return storage.DownloadLink{}, err
	}
	return storage.DownloadLink{URL: "https://cdn.example.test/film", UserAgent: ref.UserAgent, CallerUserAgent: true, DirectOnly: true, CacheUntil: time.Now().Add(time.Minute)}, nil
}
func TestPathRetryNeverRelaysMediaAndExhaustionFailsClosed(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var paths, cdnCalls atomic.Int32
		client := officialFixtureClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			if paths.Add(1) == 1 || fail {
				<-r.Context().Done()
				return
			}
			io.WriteString(w, fileResponse)
		}), 100*time.Millisecond)
		cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cdnCalls.Add(1)
			if r.Header.Get("Range") != "bytes=0-1023" {
				t.Error("unbounded media relay")
			}
			w.Header().Set("Content-Type", "video/x-matroska")
			w.Header().Set("Content-Range", "bytes 0-15/16")
			w.WriteHeader(206)
			io.WriteString(w, "0123456789abcdef")
		}))
		func() {
			defer cdn.Close()
			u, _ := url.Parse(cdn.URL)
			tr := cdn.Client().Transport.(*http.Transport).Clone()
			defer tr.CloseIdleConnections()
			engine := playback.New(retryPlaybackResolver{client}, playback.Options{ProxyFallback: true, ResolveTimeout: 2 * time.Second, Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}}, Client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				clone := r.Clone(r.Context())
				copyURL := *r.URL
				clone.URL = &copyURL
				clone.URL.Scheme, clone.URL.Host = u.Scheme, u.Host
				return tr.RoundTrip(clone)
			})}})
			request := httptest.NewRequest("HEAD", "https://media.example.test/strm", nil)
			request.Header.Set("User-Agent", "OwnedPlayer/1")
			response := httptest.NewRecorder()
			engine.Serve(response, request, storage.FileRef{})
			if fail {
				if response.Code != 504 || cdnCalls.Load() != 0 || response.Header().Get("Location") != "" {
					t.Fatal("failed retry did not fail closed", response.Code, cdnCalls.Load())
				}
			} else if response.Code != 302 || response.Header().Get("Location") != "https://cdn.example.test/film" || cdnCalls.Load() != 1 || response.Body.Len() != 0 {
				t.Fatal("successful retry became relay", response.Code, cdnCalls.Load(), response.Body.Len())
			}
			if response.Header().Get("X-Media-Delivery") == "proxy" {
				t.Fatal("retry changed original-CDN policy")
			}
		}()
	}
}
