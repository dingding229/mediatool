package linksource

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

const DefaultUserAgent = "MediaTool-NativeProbe/1.0"

var admission = struct {
	sync.Mutex
	active, limit int
	after         time.Time
}{}

type Source struct {
	deps   Dependencies
	config Config
	logger *slog.Logger
}

// ConfigureConcurrency is process-wide, independent of ffx/player concurrency.
// Reconfiguration is allowed only when no link acquisition is active.
func ConfigureConcurrency(limit int) error {
	admission.Lock()
	defer admission.Unlock()
	if limit < 1 || limit > 2 || admission.active != 0 {
		return fail(Permanent, Mismatch, 0)
	}
	admission.limit = limit
	return nil
}
func New(d Dependencies, c Config, logger *slog.Logger) (*Source, error) {
	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}
	if strings.TrimSpace(c.UserAgent) != c.UserAgent || !utf8.ValidString(c.UserAgent) || len(c.UserAgent) > 512 || strings.ContainsFunc(c.UserAgent, unicode.IsControl) || c.ResolveTimeout < 0 || c.ResolveTimeout > requestbudget.MaxConfiguredResolve || d.Repository == nil || d.Reader == nil || d.Parser == nil || d.Resolver == nil || d.Policy == nil {
		return nil, fail(Permanent, Mismatch, 0)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Source{d, c, logger}, nil
}
func (s *Source) Fetch(parent context.Context, assetID string) (ticket *Ticket, err error) {
	if s == nil || parent == nil || !validID(assetID) {
		return nil, fail(Permanent, NotExposable, 0)
	}
	ctx, cancel := context.WithTimeout(parent, requestbudget.EffectiveResolve(s.config.ResolveTimeout))
	defer cancel()
	if ctx.Err() != nil {
		return nil, classify(ctx, ctx.Err())
	}
	started := time.Now()
	defer func() { s.stage(assetID, "total", started, err) }()
	release, permitErr := takeReservation(ctx)
	if permitErr != nil {
		return nil, permitErr
	}
	defer release()
	ctx = context.WithValue(ctx, probeKey{}, true)
	// Replace any inherited observer: even a caller must not accidentally attach
	// ordinary request/URL logs to the background resolver. R1-F emits numbers only.
	ctx = storage.WithResolveObserver(ctx, func(event storage.ResolveObservation) {
		fields := []any{"assetId", assetID, "stage", string(event.Stage), "durationMs", event.DurationMillis, "result", event.Outcome}
		if h := event.HTTPTrace; h != nil {
			fields = append(fields, "dnsMs", h.DNSMillis, "tcpMs", h.TCPMillis, "tlsMs", h.TLSMillis, "firstByteMs", h.FirstByteMillis, "httpTotalMs", h.TotalMillis, "attempt", h.Attempt, "connectionReused", h.ConnectionReused)
		}
		s.logger.Info("Probe link stage", fields...)
	})
	load := time.Now()
	e, loadErr := s.deps.Repository.Load(ctx, assetID)
	s.stage(assetID, "evidence", load, loadErr)
	if loadErr != nil {
		return nil, classify(ctx, loadErr)
	}
	if e.AssetID != assetID || len(e.Digest) != sha256.Size {
		return nil, fail(Manual, Mismatch, 0)
	}
	read := time.Now()
	body, readErr := s.deps.Reader.Read(ctx, e.Output)
	s.stage(assetID, "strm_read", read, readErr)
	if readErr != nil {
		return nil, classify(ctx, readErr)
	}
	if len(body) > 8192 || !utf8.Valid(body) {
		return nil, fail(Manual, Mismatch, 0)
	}
	digest := sha256.Sum256(body)
	if subtle.ConstantTimeCompare(digest[:], e.Digest) != 1 {
		return nil, fail(Manual, Mismatch, 0)
	}
	raw := strings.TrimSpace(string(body))
	ref, parseErr := s.deps.Parser.Parse(ctx, raw)
	if parseErr != nil {
		return nil, classify(ctx, parseErr)
	}
	u, parseErr := url.Parse(raw)
	if parseErr != nil {
		return nil, fail(Manual, Mismatch, 0)
	}
	q := u.Query()
	if ref.Path != e.Path || q.Get("path") != e.Path || ref.AssetID != "" && ref.AssetID != assetID || q.Get("assetId") != "" && q.Get("assetId") != assetID || q.Get("pickCode") != "" && q.Get("pickCode") != e.PickCode {
		return nil, fail(Manual, Mismatch, 0)
	}
	ref.AssetID, ref.Path, ref.PickCode, ref.ExpectedSize, ref.UserAgent = assetID, e.Path, e.PickCode, e.Size, s.config.UserAgent
	resolve := time.Now()
	link, resolveErr := s.deps.Resolver.Resolve(ctx, ref)
	s.stage(assetID, "resolve", resolve, resolveErr)
	if resolveErr != nil {
		failure := classify(ctx, resolveErr)
		var f *Failure
		if errors.As(failure, &f) && f.Reason == Limited {
			admission.Lock()
			admission.after = time.Now().Add(f.RetryAfter)
			admission.Unlock()
		}
		return nil, failure
	}
	if link.UserAgent != "" && link.UserAgent != ref.UserAgent || len(link.Headers) > 0 {
		return nil, fail(Manual, UAMismatch, 0)
	}
	target, targetErr := url.Parse(link.URL)
	if targetErr != nil || strings.EqualFold(target.Hostname(), u.Hostname()) {
		return nil, fail(Manual, UnsafeTarget, 0)
	}
	if checkErr := s.deps.Policy.Check(ctx, link.URL); checkErr != nil {
		if ctx.Err() != nil {
			return nil, classify(ctx, ctx.Err())
		}
		return nil, fail(Manual, UnsafeTarget, 0)
	}
	// Verify the evidence again after external I/O; an asset may disappear/change
	// during lookup. Do not return a link for a replaced binding or STRM file.
	latest, checkErr := s.deps.Repository.Load(ctx, assetID)
	if checkErr != nil {
		return nil, classify(ctx, checkErr)
	}
	if latest.AssetID != assetID || latest.PickCode != e.PickCode || latest.Revision != e.Revision || latest.Path != e.Path || latest.Output != e.Output || latest.Size != e.Size || subtle.ConstantTimeCompare(latest.Digest, e.Digest) != 1 {
		return nil, fail(Manual, Mismatch, 0)
	}
	body, readErr = s.deps.Reader.Read(ctx, e.Output)
	if readErr != nil {
		return nil, classify(ctx, readErr)
	}
	digest = sha256.Sum256(body)
	if subtle.ConstantTimeCompare(digest[:], e.Digest) != 1 {
		return nil, fail(Manual, Mismatch, 0)
	}
	if ctx.Err() != nil {
		return nil, classify(ctx, ctx.Err())
	}
	expiry := time.Now().Add(45 * time.Second)
	if !link.ExpiresAt.IsZero() {
		expiry = minTime(expiry, link.ExpiresAt)
	}
	if !link.CacheUntil.IsZero() {
		expiry = minTime(expiry, link.CacheUntil)
	}
	if time.Now().After(expiry) {
		return nil, fail(Retryable, Unavailable, 5*time.Second)
	}
	return &Ticket{state: &ticketState{url: link.URL, ua: ref.UserAgent, expires: expiry}}, nil
}
func minTime(a, b time.Time) time.Time { return requestbudget.Earlier(a, b) }
func classify(ctx context.Context, err error) error {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, storage.ErrRequestTimeout) {
		return fail(Retryable, Timeout, 10*time.Second)
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return fail(Retryable, Timeout, 10*time.Second)
	}
	var api *open115.APIError
	if errors.Is(err, open115.ErrRateLimited) || errors.As(err, &api) && api.RateLimited() {
		return fail(Retryable, Limited, time.Minute)
	}
	if errors.Is(err, open115.ErrAuthorization) || errors.Is(err, playbackgrants.ErrDenied) || errors.Is(err, storage.ErrUnauthenticated) || errors.Is(err, storage.ErrForbidden) || errors.As(err, &api) && (api.ExpiredAccess() || api.TerminalRefresh()) {
		return fail(Manual, Authorization, 0)
	}
	if errors.Is(err, storage.ErrNotFound) {
		return fail(Permanent, NotExposable, 0)
	}
	return fail(Retryable, Unavailable, 15*time.Second)
}
func (s *Source) stage(asset, phase string, start time.Time, err error) {
	result := "ok"
	if err != nil {
		var f *Failure
		if errors.As(err, &f) {
			result = string(f.Reason)
		} else {
			result = "failed"
		}
	}
	s.logger.Info("Probe link stage", "assetId", asset, "stage", phase, "durationMs", time.Since(start).Milliseconds(), "result", result)
}
func validID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
