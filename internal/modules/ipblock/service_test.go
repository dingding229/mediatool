package ipblock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	lastBatch           string
	ambiguousCommit     bool
	ambiguousChange     bool
	mu                  sync.Mutex
	p                   Policy
	failLoad, failFlush bool
	loads               int
}

func (m *memoryStore) Load(context.Context) (Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	if m.failLoad {
		return Policy{}, ErrUnavailable
	}
	p := m.p
	p.Rules = append([]Rule{}, p.Rules...)
	return p, nil
}
func (m *memoryStore) Change(_ context.Context, in Mutation) (Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in.Revision != m.p.Revision {
		return Policy{}, ErrConflict
	}
	if in.Delete {
		for i, r := range m.p.Rules {
			if r.ID == in.ID {
				m.p.Rules = append(m.p.Rules[:i], m.p.Rules[i+1:]...)
				break
			}
		}
	} else {
		found := false
		for i, r := range m.p.Rules {
			if r.ID == in.ID {
				m.p.Rules[i].Network = in.Network
				found = true
			}
		}
		if !found {
			m.p.Rules = append(m.p.Rules, Rule{ID: fmt.Sprint(len(m.p.Rules) + 1), Network: in.Network})
		}
	}
	m.p.Revision++
	p := m.p
	p.Rules = append([]Rule{}, p.Rules...)
	if m.ambiguousChange {
		m.ambiguousChange = false
		return p, ErrCommitUnknown
	}
	return p, nil
}
func (m *memoryStore) AddHits(_ context.Context, batchID string, h map[string]Delta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failFlush {
		return errors.New("offline")
	}
	if batchID == m.lastBatch {
		return nil
	}
	m.lastBatch = batchID
	for i, r := range m.p.Rules {
		if d, ok := h[r.ID]; ok {
			m.p.Rules[i].HitCount += d.Count
			v := d.Last
			m.p.Rules[i].LastHitAt = &v
		}
	}
	if m.ambiguousCommit {
		m.ambiguousCommit = false
		return errors.New("commit acknowledgement lost")
	}

	return nil
}
func testService(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	m := &memoryStore{p: Policy{Revision: 1, Rules: []Rule{{ID: "v4", Network: "203.0.113.9/32"}, {ID: "net", Network: "8.8.8.0/24"}, {ID: "v6", Network: "2001:db8::/32"}, {ID: "exact", Network: "8.8.8.8/32"}}}}
	s := New(m, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e := s.Reload(context.Background()); e != nil {
		t.Fatal(e)
	}
	return s, m
}
func TestNetworkAndMatching(t *testing.T) {
	for _, raw := range []string{"", "broken", "203.0.113.4/24", "0.0.0.0/0", "::/0", "fe80::1%eth0", "224.0.0.1"} {
		if _, e := Network(raw); e == nil {
			t.Fatal(raw)
		}
	}
	s, m := testService(t)
	for _, c := range []struct{ ip, id string }{{"203.0.113.9", "v4"}, {"8.8.8.7", "net"}, {"8.8.8.8", "exact"}, {"::ffff:8.8.8.8", "exact"}, {"2001:db8::5", "v6"}, {"1.1.1.1", ""}, {"2606:4700::1111", ""}} {
		v, e := s.Evaluate(netip.MustParseAddr(c.ip), false)
		if e != nil || v.RuleID != c.id {
			t.Fatalf("%s: %+v %v", c.ip, v, e)
		}
	}
	_ = s.Flush(context.Background())
	for _, r := range m.p.Rules {
		if r.HitCount != 0 {
			t.Fatal("dry run counted")
		}
	}
	if _, e := compile(Policy{Rules: make([]Rule, MaxRules+1)}); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}
func TestImmediateChangesCountersFailureAndDeletedRulesStayDeleted(t *testing.T) {
	s, m := testService(t)
	ctx := context.Background()
	ip := netip.MustParseAddr("203.0.113.9")
	for i := 0; i < 100; i++ {
		_, _ = s.Evaluate(ip, true)
	}
	m.failFlush = true
	if s.Flush(ctx) == nil || !s.Status().CounterWarning {
		t.Fatal("failed counts discarded")
	}
	m.failFlush = false
	if e := s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if m.p.Rules[0].HitCount != 100 || m.p.Rules[0].LastHitAt == nil {
		t.Fatal(m.p)
	}
	_, _ = s.Evaluate(ip, true)
	p, e := s.Change(ctx, Mutation{ID: "v4", Delete: true, Revision: 1, Confirm: true})
	if e != nil || p.Revision != 2 {
		t.Fatal(e)
	}
	v, _ := s.Evaluate(ip, true)
	if v.Matched {
		t.Fatal("delete not immediate")
	}
	if e = s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if len(m.p.Rules) != 3 {
		t.Fatal("flush resurrected a rule")
	}
	if _, e = s.Change(ctx, Mutation{Revision: 1, Network: "1.1.1.1", Confirm: true}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e = s.Change(ctx, Mutation{Revision: 2, Network: "1.1.1.1", Confirm: true}); e != nil {
		t.Fatal(e)
	}
	if v, e := s.Evaluate(netip.MustParseAddr("1.1.1.1"), false); e != nil || !v.Matched {
		t.Fatal(v, e)
	}
}
func TestFailClosedRetryAutomaticRecoveryAndRestoreBarrier(t *testing.T) {
	m := &memoryStore{p: Policy{Revision: 1, Rules: []Rule{}}, failLoad: true}
	s := New(m, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	defer func() { cancel(); <-done }()
	if _, e := s.Evaluate(netip.MustParseAddr("8.8.8.8"), false); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	deadline := time.Now().Add(2 * time.Second)
	for s.Status().RetrySeconds == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Status().RetrySeconds != 1 {
		t.Fatal(s.Status())
	}
	deadline = time.Now().Add(2 * time.Second)
	for s.Status().RetrySeconds != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Status().RetrySeconds != 2 {
		t.Fatal("retry did not back off", s.Status())
	}
	m.mu.Lock()
	m.failLoad = false
	m.mu.Unlock()
	deadline = time.Now().Add(3 * time.Second)
	for !s.Status().Available && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !s.Status().Available {
		t.Fatal("requires restart")
	}
	resume := s.Suspend()
	if _, e := s.Evaluate(netip.MustParseAddr("8.8.8.8"), false); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	resume()
	resume()
	if !s.Status().Available {
		t.Fatal("restore did not reload")
	}
}
func TestConcurrentReadChangeAndFlush(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				_, _ = s.Evaluate(netip.MustParseAddr("8.8.8.8"), true)
			}
		}()
	}
	for j := 0; j < 8; j++ {
		_ = s.Flush(ctx)
	}
	wg.Wait()
	_ = s.Flush(ctx)
}

func BenchmarkMatch10000Rules(b *testing.B) {
	p := Policy{Revision: 1, Rules: make([]Rule, 10000)}
	for i := range p.Rules {
		p.Rules[i] = Rule{ID: fmt.Sprint(i), Network: fmt.Sprintf("10.%d.%d.1/32", i/256, i%256)}
	}
	v, e := compile(p)
	if e != nil {
		b.Fatal(e)
	}
	s := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.current.Store(v)
	ip := netip.MustParseAddr("10.3.4.1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if m, e := s.Evaluate(ip, false); e != nil || !m.Matched {
			b.Fatal(m, e)
		}
	}
}

func TestAmbiguousCommitsNeverDoubleCountOrKeepAStaleAllowSnapshot(t *testing.T) {
	s, m := testService(t)
	ctx := context.Background()
	ip := netip.MustParseAddr("203.0.113.9")
	_, _ = s.Evaluate(ip, true)
	m.ambiguousCommit = true
	if s.Flush(ctx) == nil {
		t.Fatal("expected acknowledgement failure")
	}
	if m.p.Rules[0].HitCount != 1 {
		t.Fatal(m.p)
	}
	_, _ = s.Evaluate(ip, true)
	if e := s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if m.p.Rules[0].HitCount != 1 {
		t.Fatal("uncertain batch counted twice")
	}
	if e := s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if m.p.Rules[0].HitCount != 2 {
		t.Fatal("new incoming batch lost")
	}
	m.ambiguousChange = true
	if _, e := s.Change(ctx, Mutation{Network: "1.1.1.1", Revision: 1, Confirm: true}); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e := s.Evaluate(netip.MustParseAddr("1.1.1.1"), false); !errors.Is(e, ErrUnavailable) {
		t.Fatal("stale allow snapshot remained active", e)
	}
	if e := s.Reload(ctx); e != nil {
		t.Fatal(e)
	}
	if v, e := s.Evaluate(netip.MustParseAddr("1.1.1.1"), false); e != nil || !v.Matched {
		t.Fatal(v, e)
	}
}
