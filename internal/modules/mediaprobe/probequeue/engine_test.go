package probequeue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/linksource"
)

const engineAsset = "12345678-1234-4234-8234-123456789abc"
const engineSecret = "https://synthetic.example/file?SIGN=NEVER_LOG_RAW"

type engineStore struct {
	on                      atomic.Bool
	mu                      sync.Mutex
	claims                  int
	probePermit, linkPermit atomic.Bool
	failure                 *Failure
	result                  Result
	committed               bool
}

func (s *engineStore) Enabled(context.Context) (bool, error) { return s.on.Load(), nil }
func (s *engineStore) Claim(context.Context, string, time.Duration) (*Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims++
	if !s.probePermit.Load() || !s.linkPermit.Load() {
		panic("lease before permits")
	}
	return &Lease{AssetID: engineAsset}, nil
}
func (s *engineStore) Succeed(_ context.Context, _ Lease, r Result) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = r
	s.committed = true
	return true, nil
}
func (s *engineStore) Fail(_ context.Context, _ Lease, f Failure) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = &f
	return true, nil
}

type fakeTicket struct{}

func (fakeTicket) Claim() (string, string, error) { return engineSecret, "SyntheticProbe/23", nil }

type engineLinks struct {
	s     *engineStore
	block <-chan struct{}
	fail  error
	after func()
}

func (l engineLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	if l.block != nil {
		select {
		case <-l.block:
		case <-ctx.Done():
			return ctx, nil, ctx.Err()
		}
	}
	l.s.linkPermit.Store(true)
	var once sync.Once
	return ctx, func() { once.Do(func() { l.s.linkPermit.Store(false) }) }, nil
}
func (l engineLinks) Fetch(context.Context, string) (Ticket, error) {
	if l.after != nil {
		l.after()
	}
	return fakeTicket{}, l.fail
}

type engineProbe struct {
	s     *engineStore
	fail  error
	run   func(context.Context)
	calls atomic.Int32
}

func (p *engineProbe) Reserve(ctx context.Context) (context.Context, func(), error) {
	p.s.probePermit.Store(true)
	var once sync.Once
	return ctx, func() { once.Do(func() { p.s.probePermit.Store(false) }) }, nil
}
func (p *engineProbe) Probe(ctx context.Context, url string, o ProbeOptions) ([]byte, error) {
	p.calls.Add(1)
	if url != engineSecret || o.UserAgent != "SyntheticProbe/23" {
		return nil, errors.New("UA drift")
	}
	if p.run != nil {
		p.run(ctx)
	}
	if p.fail != nil {
		return nil, p.fail
	}
	return []byte(`{"format":{"duration":"12.25","filename":"` + engineSecret + `"},"SECRET":"NEVER_LOG_RAW"}`), ctx.Err()
}
func engineFixture() (Engine, *engineStore, *engineProbe, *bytes.Buffer) {
	s := &engineStore{}
	s.on.Store(true)
	p := &engineProbe{s: s}
	log := &bytes.Buffer{}
	e := Engine{Store: s, Links: engineLinks{s: s}, Probe: p, Logger: slog.New(slog.NewJSONHandler(log, nil))}
	return e, s, p, log
}
func engineJob() jobs.Job {
	raw, err := json.Marshal(map[string]string{"assetId": engineAsset})
	if err != nil {
		panic(err)
	}
	return jobs.Job{Payload: raw}
}
func TestEnginePermitsPrecedeLeaseAndRawJSONIsDiscarded(t *testing.T) {
	e, s, _, log := engineFixture()
	gate := make(chan struct{})
	e.Links = engineLinks{s: s, block: gate}
	done := make(chan error, 1)
	go func() { done <- e.Process(context.Background(), engineJob()) }()
	time.Sleep(25 * time.Millisecond)
	s.mu.Lock()
	claims := s.claims
	s.mu.Unlock()
	if claims != 0 {
		t.Fatal("waiting worker occupied lease")
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s.result.DurationTicks != 122500000 || !s.committed || s.failure != nil {
		t.Fatal("placeholder result", s.result)
	}
	if strings.Contains(log.String(), "NEVER_LOG_RAW") || strings.Contains(log.String(), "https://") {
		t.Fatal("raw JSON/URL logged")
	}
	if s.probePermit.Load() || s.linkPermit.Load() {
		t.Fatal("resource leak")
	}
}
func TestEngineFailuresSucceedWakeAndStoreOnlyCodes(t *testing.T) {
	for _, tt := range []struct {
		name        string
		link, probe error
		class       linksource.Class
		reason      string
	}{
		{"limited", &linksource.Failure{Class: linksource.Retryable, Reason: linksource.Limited, RetryAfter: time.Minute}, nil, linksource.Retryable, "rate_limited"},
		{"manual", &linksource.Failure{Class: linksource.Manual, Reason: linksource.Authorization}, nil, linksource.Manual, "authorization"},
		{"permanent", &linksource.Failure{Class: linksource.Permanent, Reason: linksource.FileMissing}, nil, linksource.Permanent, "file_missing"},
		{"process", nil, errors.New(engineSecret), linksource.Retryable, "process_failed"},
		{"unavailable", nil, ffx.ErrUnavailable, linksource.Manual, "unavailable"},
		{"timeout", nil, context.DeadlineExceeded, linksource.Retryable, "timeout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e, s, p, log := engineFixture()
			e.Links = engineLinks{s: s, fail: tt.link}
			p.fail = tt.probe
			if err := e.Process(context.Background(), engineJob()); err != nil {
				t.Fatal("wake must succeed")
			}
			if s.failure == nil || s.failure.Class != tt.class || s.failure.Reason != tt.reason || s.committed {
				t.Fatal("failure mapping", s.failure)
			}
			if strings.Contains(log.String(), "https://") || strings.Contains(log.String(), "NEVER_LOG_RAW") {
				t.Fatal("secret leak")
			}
			if s.probePermit.Load() || s.linkPermit.Load() {
				t.Fatal("permit leaked")
			}
		})
	}
}
func TestEngineDisabledAndMidFlightDisable(t *testing.T) {
	e, s, p, _ := engineFixture()
	s.on.Store(false)
	if err := e.Process(context.Background(), engineJob()); err != nil || s.claims != 0 || p.calls.Load() != 0 {
		t.Fatal("disabled execution")
	}
	s.on.Store(true)
	e.Links = engineLinks{s: s, after: func() { s.on.Store(false) }}
	if err := e.Process(context.Background(), engineJob()); err != nil || p.calls.Load() != 0 || s.failure == nil {
		t.Fatal("disabled between link and probe")
	}
	e, s, p, _ = engineFixture()
	started := make(chan struct{})
	p.run = func(ctx context.Context) { close(started); <-ctx.Done() }
	done := make(chan error, 1)
	go func() { done <- e.Process(context.Background(), engineJob()) }()
	<-started
	s.on.Store(false)
	select {
	case err := <-done:
		if err != nil || s.failure == nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("disable did not cancel running probe")
	}
}
func TestEngineCancelledResourceWaitLeavesNoLease(t *testing.T) {
	e, s, _, _ := engineFixture()
	e.Links = engineLinks{s: s, block: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := e.Process(ctx, engineJob()); err != nil || s.claims != 0 || s.probePermit.Load() {
		t.Fatal("canceled wait claimed/leaked lease")
	}
}
func TestResultParserDurationCompatibility(t *testing.T) {
	parser := ResultParser{}
	for raw, want := range map[string]int64{`{}`: 0, `{"format":{"duration":"N/A"}}`: 0, `{"format":{"duration":0}}`: 0, `{"format":{"duration":"1.25"}}`: 12500000, `{"format":{"duration":1.5,"filename":"` + engineSecret + `"}}`: 15000000} {
		r, err := parser.Parse([]byte(raw))
		if err != nil || r.DurationTicks != want {
			t.Fatal(raw, r, err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"format":[]}`, `{"format":{"duration":true}}`, `{"format":{"duration":"-1"}}`, `{"format":{"duration":"NaN"}}`, `{"format":{"duration":"Inf"}}`, `{"format":{"duration":"9999999999999999"}}`, `{`, strings.Repeat("x", (8<<20)+1)} {
		if _, err := parser.Parse([]byte(raw)); err == nil {
			t.Fatal("invalid raw result accepted")
		}
	}
}
