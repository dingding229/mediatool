package httpapi

import (
	"bufio"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"media-tool/internal/platform/auth"
)

func TestRP31JobStreamBudgets(t *testing.T) {
	var budget jobStreamBudget
	var releases []func()
	for i := 0; i < maxJobStreams; i++ {
		release := budget.acquire([sha256.Size]byte{byte(i / maxJobStreamsPerSession)})
		if release == nil {
			t.Fatal("budget denied a slot", i)
		}
		releases = append(releases, release)
		if i == maxJobStreamsPerSession-1 && budget.acquire([sha256.Size]byte{}) != nil {
			t.Fatal("accepted excess session stream")
		}
	}
	if budget.acquire([sha256.Size]byte{255}) != nil {
		t.Fatal("accepted excess global stream")
	}
	for _, release := range releases {
		release()
	}
	if budget.active != 0 || len(budget.bySession) != 0 {
		t.Fatal("stream budget leaked slots")
	}
}

type deadlineJobWriter struct {
	*httptest.ResponseRecorder
	deadlines    []time.Time
	flushTimeout bool
}

func (w *deadlineJobWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (w *deadlineJobWriter) FlushError() error {
	if w.flushTimeout {
		return os.ErrDeadlineExceeded
	}
	w.Flush()
	return nil
}

func TestRP31JobHeartbeatDeadline(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		w := &deadlineJobWriter{ResponseRecorder: httptest.NewRecorder(), flushTimeout: timeout}
		before := time.Now()
		if writeJobHeartbeat(w, http.NewResponseController(w)) == timeout {
			t.Fatal("heartbeat did not stop on flush timeout")
		}
		if len(w.deadlines) == 0 || w.deadlines[0].Before(before.Add(jobEventWriteTimeout)) || w.deadlines[0].After(time.Now().Add(jobEventWriteTimeout)) || w.Body.String() != "event: tick\ndata: {}\n\n" {
			t.Fatal("missing per-frame deadline or tick")
		}
		if !timeout && (len(w.deadlines) != 2 || !w.deadlines[1].IsZero()) {
			t.Fatal("successful flush did not release deadline")
		}
	}
}

func TestRP31JobStreamHTTPCookieHeadAndSessionLimit(t *testing.T) {
	c := cfg()
	token, err := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(New(c, Dependencies{AuthSessions: testSessionReader{}}))
	defer s.Close()
	client := s.Client()
	client.Timeout = 5 * time.Second
	call := func(method string, authenticated bool) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, s.URL+"/api/v1/jobs/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		if authenticated {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	if r := call("GET", false); r.StatusCode != 401 {
		t.Fatal("anonymous SSE accepted", r.StatusCode)
	}
	for i := 0; i < maxJobStreamsPerSession; i++ {
		r := call("GET", true)
		defer r.Body.Close()
		if r.StatusCode != 200 || r.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatal("unexpected stream response", r.StatusCode)
		}
		if tick, err := bufio.NewReader(r.Body).ReadString('\n'); err != nil || tick != "event: tick\n" {
			t.Fatal("expected first heartbeat", err)
		}
	}
	if r := call("GET", true); r.StatusCode != 429 || r.Header.Get("Retry-After") != "3" {
		t.Fatal("session cap not enforced", r.StatusCode)
	}
	if r := call("HEAD", true); r.StatusCode != 200 || r.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("authenticated HEAD should not reserve a stream", r.StatusCode)
	}
}
