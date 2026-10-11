package emby

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type failingTransport func(*http.Request) (*http.Response, error)

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProxyFailureClassesNeverExposeRawDetails(t *testing.T) {
	target, _ := url.Parse("https://emby.fixture")
	for _, test := range []struct {
		name   string
		err    error
		reason string
	}{{"closed", io.EOF, "upstream_closed"}, {"partial", io.ErrUnexpectedEOF, "upstream_closed"}, {"timeout", context.DeadlineExceeded, "upstream_timeout"}, {"other", errors.New("NEVER-LOG-CREDENTIAL url=SECRET"), "upstream_unavailable"}} {
		t.Run(test.name, func(t *testing.T) {
			private := &url.Error{Op: "Get", URL: "https://emby.fixture?api_key=NEVER-LOG-TOKEN", Err: test.err}
			proxy := newProxyWithTransport(target, nil, nil, nil, failingTransport(func(*http.Request) (*http.Response, error) { return nil, private }))
			writer := httptest.NewRecorder()
			proxy.ServeHTTP(writer, httptest.NewRequest("GET", "/emby/videos/11/hls1/main/0.ts", nil))
			if writer.Code != 502 || writer.Header().Get("X-Media-Gateway-Reason") != test.reason {
				t.Fatal("wrong error class", writer.Code, writer.Header())
			}
			if strings.Contains(writer.Body.String(), "NEVER-LOG") || strings.Contains(writer.Header().Get("X-Media-Gateway-Reason"), "SECRET") {
				t.Fatal("upstream credential/detail exposed")
			}
		})
	}
}
func TestCancelledFragmentIsAbortedNotReportedAsDelivered502(t *testing.T) {
	target, _ := url.Parse("https://emby.fixture")
	ctx, cancel := context.WithCancel(context.Background())
	proxy := newProxyWithTransport(target, nil, nil, nil, failingTransport(func(*http.Request) (*http.Response, error) { cancel(); return nil, io.EOF }))
	writer := httptest.NewRecorder()
	func() {
		defer func() {
			if got := recover(); got != http.ErrAbortHandler {
				t.Fatal("cancel not aborted", got)
			}
		}()
		proxy.ServeHTTP(writer, httptest.NewRequest("GET", "/emby/videos/11/hls1/main/0.ts", nil).WithContext(ctx))
	}()
	if writer.Body.Len() != 0 || writer.Header().Get("X-Media-Gateway-Reason") != "request_cancelled" {
		t.Fatal("cancelled caller received gateway error body")
	}
	// A cancelled error with a LIVE incoming context is not proof the player left.
	if got := proxyFailureReason(context.Background(), context.Canceled); got != "upstream_unavailable" {
		t.Fatal("upstream cancellation incorrectly blamed on caller")
	}
}
