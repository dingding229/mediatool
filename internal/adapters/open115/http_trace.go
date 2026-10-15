package open115

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http/httptrace"
	"sync"
	"syscall"
	"time"

	"media-tool/internal/modules/storage"
)

const pathRetryBackoff = 100 * time.Millisecond

var errTransportReset = errors.New("official provider connection reset")

// Retain only sanitized error identity, never the URL-bearing transport error.
type noHTTPResponseError struct{ cause error }

func (e *noHTTPResponseError) Error() string { return e.cause.Error() }
func (e *noHTTPResponseError) Unwrap() error { return e.cause }

func retryableNoResponse(err error) bool {
	var noResponse *noHTTPResponseError
	return errors.As(err, &noResponse) && (errors.Is(err, storage.ErrRequestTimeout) || errors.Is(err, errTransportReset))
}

func enoughForRetry(ctx context.Context, requestTimeout time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return ok && ctx.Err() == nil && requestTimeout > 0 && time.Until(deadline) > requestTimeout+pathRetryBackoff
}

// httptrace callbacks may run concurrently, including a late failed dial after
// a winning Happy Eyeballs connection. No host/address/header values leave this
// object, and late callbacks cannot change an already-emitted observation.
type httpTrace struct {
	mu                sync.Mutex
	started, dns, tls time.Time
	connect           map[string]time.Time
	finished          bool
	timing            storage.HTTPTraceTimings
}

func newHTTPTrace(started time.Time, attempt int) *httpTrace {
	return &httpTrace{started: started, connect: map[string]time.Time{}, timing: storage.HTTPTraceTimings{
		Attempt: attempt, DNSMillis: -1, TCPMillis: -1, TLSMillis: -1, FirstByteMillis: -1,
	}}
}
func (t *httpTrace) update(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		fn()
	}
}
func elapsed(start time.Time) int64 { return max(0, time.Since(start).Milliseconds()) }

func (t *httpTrace) traceContext(ctx context.Context) context.Context {
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { t.update(func() { t.dns = time.Now() }) },
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.update(func() {
				if !t.dns.IsZero() {
					t.timing.DNSMillis = elapsed(t.dns)
				}
			})
		},
		ConnectStart: func(network, address string) { t.update(func() { t.connect[network+address] = time.Now() }) },
		ConnectDone: func(network, address string, _ error) {
			t.update(func() {
				if start, ok := t.connect[network+address]; ok {
					t.timing.TCPMillis = max(t.timing.TCPMillis, elapsed(start))
					delete(t.connect, network+address)
				}
			})
		},
		TLSHandshakeStart: func() { t.update(func() { t.tls = time.Now() }) },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.update(func() {
				if !t.tls.IsZero() {
					t.timing.TLSMillis = elapsed(t.tls)
				}
			})
		},
		GotConn:              func(info httptrace.GotConnInfo) { t.update(func() { t.timing.ConnectionReused = info.Reused }) },
		GotFirstResponseByte: func() { t.update(func() { t.timing.FirstByteMillis = elapsed(t.started) }) },
	})
}
func (t *httpTrace) receivedResponse() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.timing.ResponseReceived || t.timing.FirstByteMillis >= 0
}
func (t *httpTrace) snapshot() *storage.HTTPTraceTimings {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	out := t.timing
	out.TotalMillis = elapsed(t.started)
	return &out
}
func connectionReset(err error) bool { return errors.Is(err, syscall.ECONNRESET) }
