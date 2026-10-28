package ipblock

import (
	"context"
	"errors"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"net/netip"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type snapshot struct {
	policy  Policy
	entries map[netip.Prefix]Rule
	lengths []int
}
type pendingHits struct {
	id   string
	data map[string]Delta
}
type Service struct {
	pending        *pendingHits
	store          Store
	log            *slog.Logger
	current        atomic.Pointer[snapshot]
	mu             sync.Mutex // Serializes policy writes, reloads, flushes and restore barriers, never request matching.
	hitsMu         sync.Mutex
	hits           map[string]Delta
	suspended      atomic.Bool
	counterWarning atomic.Bool
	retrySeconds   atomic.Int64
}

func New(db Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: db, log: log, hits: map[string]Delta{}}
}
func compile(p Policy) (*snapshot, error) {
	s := &snapshot{policy: p, entries: map[netip.Prefix]Rule{}}
	lengths := map[int]bool{}
	if len(p.Rules) > MaxRules {
		return nil, ErrLimit
	}
	for _, r := range p.Rules {
		p, e := Network(r.Network)
		if e != nil {
			return nil, e
		}
		s.entries[p] = r
		lengths[p.Bits()] = true
	}
	for n := range lengths {
		s.lengths = append(s.lengths, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(s.lengths)))
	return s, nil
}
func (s *Service) reload(ctx context.Context) error {
	p, e := s.store.Load(ctx)
	if e != nil {
		return e
	}
	v, e := compile(p)
	if e != nil {
		return e
	}
	s.current.Store(v)
	s.retrySeconds.Store(0)
	return nil
}
func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.suspended.Load() {
		return ErrUnavailable
	}
	return s.reload(ctx)
}
func (s *Service) Status() Status {
	v := Status{Available: s != nil && s.current.Load() != nil && !s.suspended.Load()}
	if s == nil {
		v.Message = ErrUnavailable.Error()
		return v
	}
	v.RetrySeconds = s.retrySeconds.Load()
	v.CounterWarning = s.counterWarning.Load()
	if !v.Available {
		v.Message = ErrUnavailable.Error()
	}
	return v
}

// Evaluate is shared by live admission and administrator-only dry runs. A dry run never records hits.
func (s *Service) Evaluate(ip netip.Addr, count bool, fields ...any) (Match, error) {
	if s == nil || s.suspended.Load() {
		return Match{}, ErrUnavailable
	}
	v := s.current.Load()
	if v == nil {
		return Match{}, ErrUnavailable
	}
	m := Match{Revision: v.policy.Revision}
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" {
		return m, nil
	}
	for _, bits := range v.lengths {
		if bits > ip.BitLen() {
			continue
		}
		if r, ok := v.entries[netip.PrefixFrom(ip, bits).Masked()]; ok {
			m.Matched, m.RuleID, m.Network = true, r.ID, r.Network
			if count {
				s.hitsMu.Lock()
				live := s.current.Load()
				if live == nil || s.suspended.Load() || live.entries[netip.MustParsePrefix(r.Network)].ID != r.ID {
					s.hitsMu.Unlock()
					break
				}
				d := s.hits[r.ID]
				if d.Count == 0 {
					s.log.Warn("媒体请求命中 IP 黑名单规则", append([]any{"module", "ipblock", "ruleId", r.ID, "network", r.Network, "clientIp", ip.String()}, fields...)...)
				}
				d.Count++
				d.Last = time.Now().UTC()
				s.hits[r.ID] = d
				s.hitsMu.Unlock()
			}
			break
		}
	}
	return m, nil
}
func (s *Service) Change(ctx context.Context, m Mutation) (Policy, error) {
	if e := m.Validate(); e != nil {
		return Policy{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.suspended.Load() {
		return Policy{}, ErrUnavailable
	}
	p, e := s.store.Change(ctx, m)
	if e != nil {
		if errors.Is(e, ErrConflict) || errors.Is(e, ErrNotFound) || errors.Is(e, ErrLimit) || errors.Is(e, ErrDuplicate) {
			return p, e
		}
		if errors.Is(e, ErrCommitUnknown) {
			s.current.Store(nil)
			s.log.Warn("IP 黑名单规则修改结果待核对，媒体请求返回 503，后台将重载数据库策略", "module", "ipblock")
		}
		return p, ErrUnavailable
	}
	v, e := compile(p)
	if e != nil {
		s.current.Store(nil)
		return p, e
	}
	s.current.Store(v)
	s.retrySeconds.Store(0)
	// Drop pending hits of deleted rules. Late readers hold at most one old rule ID per request;
	// AddHits only UPDATEs existing rows and cannot resurrect them.
	s.hitsMu.Lock()
	ids := map[string]bool{}
	for _, r := range p.Rules {
		ids[r.ID] = true
	}
	for id := range s.hits {
		if !ids[id] {
			delete(s.hits, id)
		}
	}
	s.hitsMu.Unlock()
	return p, nil
}
func (s *Service) Policy(ctx context.Context) (Policy, error) { return s.store.Load(ctx) }
func (s *Service) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.suspended.Load() {
		return nil
	}
	if s.pending == nil {
		s.hitsMu.Lock()
		batch := s.hits
		s.hits = map[string]Delta{}
		s.hitsMu.Unlock()
		if len(batch) == 0 {
			s.counterWarning.Store(false)
			return nil
		}
		s.pending = &pendingHits{id: jobs.UUID(), data: batch}
	}
	e := s.store.AddHits(ctx, s.pending.id, s.pending.data)
	s.counterWarning.Store(e != nil)
	if e == nil {
		s.pending = nil
	}

	return e
}

// Suspend is used only around business restore/compensation. No request waits for the barrier:
// media admission is unavailable; management remains reachable. Old pending counts are discarded.
func (s *Service) Suspend() func() {
	s.mu.Lock()
	s.suspended.Store(true)
	s.current.Store(nil)
	s.pending = nil
	s.counterWarning.Store(false)
	s.hitsMu.Lock()
	s.hits = map[string]Delta{}
	s.hitsMu.Unlock()
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			e := s.reload(ctx)
			s.suspended.Store(false)
			if e != nil {
				s.log.Warn("恢复后 IP 黑名单策略重载失败，媒体请求未放行，正在自动重试", "module", "ipblock")
			}
		})
	}
}
func (s *Service) Run(ctx context.Context) error {
	delay := time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	flush := time.NewTicker(5 * time.Second)
	defer flush.Stop()
	for {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.Flush(c)
			_ = s.Flush(c)
			return nil
		case <-timer.C:
			if s.current.Load() == nil && !s.suspended.Load() {
				c, cancel := context.WithTimeout(ctx, 3*time.Second)
				e := s.Reload(c)
				cancel()
				if e != nil {
					s.retrySeconds.Store(int64(delay / time.Second))
					s.log.Warn("IP 黑名单策略加载失败，媒体请求返回 503，将自动重试", "module", "ipblock", "retrySeconds", int64(delay/time.Second))
					timer.Reset(delay)
					delay = min(delay*2, time.Minute)
					continue
				}
				s.log.Info("IP 黑名单策略已加载，媒体准入自动恢复", "module", "ipblock")
				delay = time.Second
			}
			timer.Reset(time.Second)
		case <-flush.C:
			c, cancel := context.WithTimeout(ctx, 3*time.Second)
			e := s.Flush(c)
			cancel()
			if e != nil {
				s.log.Warn("IP 黑名单命中计数刷盘失败，保留增量重试；阻断不受影响", "module", "ipblock")
			}
		}
	}
}

// CoveredBy previews complete network coverage without counting hits or changing policy.
// Change rechecks this under the database policy lock; preview is never write authority.
func (s *Service) CoveredBy(candidate netip.Prefix, excludeID string) (string, error) {
	if s == nil || !s.Status().Available {
		return "", ErrUnavailable
	}
	v := s.current.Load()
	if v == nil {
		return "", ErrUnavailable
	}
	for p, r := range v.entries {
		if r.ID == excludeID && p == candidate {
			return "", nil
		}
	}
	var covering netip.Prefix
	for p, r := range v.entries {
		if r.ID != excludeID && p.Addr().BitLen() == candidate.Addr().BitLen() && p.Bits() <= candidate.Bits() && p.Contains(candidate.Addr()) && (!covering.IsValid() || p.Bits() < covering.Bits()) {
			covering = p
		}
	}
	if covering.IsValid() {
		return covering.String(), nil
	}
	return "", nil
}
