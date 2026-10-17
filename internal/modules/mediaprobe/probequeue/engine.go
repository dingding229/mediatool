package probequeue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/linksource"
)

type AttemptStore interface {
	Enabled(context.Context) (bool, error)
	Claim(context.Context, string, time.Duration) (*Lease, error)
	Succeed(context.Context, Lease, Result) (bool, error)
	Fail(context.Context, Lease, Failure) (bool, error)
}
type Ticket interface {
	Claim() (string, string, error)
}
type Links interface {
	Reserve(context.Context) (context.Context, func(), error)
	Fetch(context.Context, string) (Ticket, error)
}
type ProbeOptions struct {
	UserAgent string
	Timeout   time.Duration
}
type Probe interface {
	Reserve(context.Context) (context.Context, func(), error)
	Probe(context.Context, string, ProbeOptions) ([]byte, error)
}
type Parser interface{ Parse([]byte) (Result, error) }
type Engine struct {
	Store                        AttemptStore
	Links                        Links
	Probe                        Probe
	Parser                       Parser
	ResolveTimeout, ProbeTimeout time.Duration
	Logger                       *slog.Logger
	AfterReady                   func(context.Context, string) error
}

// Process NEVER returns a probe/provider/DB error to jobs. Durable scheduling is
// exclusively in the probe row; missing writes converge by lease recovery/sweep.
func (e Engine) Process(parent context.Context, j jobs.Job) error {
	var in struct {
		AssetID string `json:"assetId"`
	}
	if json.Unmarshal(j.Payload, &in) != nil || !validID(in.AssetID) || e.Store == nil || e.Links == nil || e.Probe == nil {
		return nil
	}
	leaseTime, err := LeaseDuration(e.ResolveTimeout, e.ProbeTimeout)
	if err != nil {
		return nil
	}
	enabled, err := e.Store.Enabled(parent)
	if err != nil || !enabled {
		return nil
	}
	wait, stop := context.WithTimeout(parent, 2*time.Minute+leaseTime)
	defer stop()
	// Both reservations are the real global ffx/linksource permits, not shadow
	// counters. No probe lease exists while either resource is unavailable.
	reserved, releaseProbe, err := e.Probe.Reserve(wait)
	if err != nil {
		return nil
	}
	defer releaseProbe()
	reserved, releaseLink, err := e.Links.Reserve(reserved)
	if err != nil {
		return nil
	}
	defer releaseLink()
	l, err := e.Store.Claim(reserved, in.AssetID, leaseTime)
	if err != nil || l == nil {
		return nil
	}
	run, cancel := context.WithTimeout(reserved, leaseTime-5*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-ticker.C:
				on, checkErr := e.Store.Enabled(run)
				if checkErr != nil || !on {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	ticket, workErr := e.Links.Fetch(run, l.AssetID)
	if ticket == nil && workErr == nil {
		workErr = ErrInvalid
	}
	releaseLink() // Fetch consumes/releases its permit; unused permits close here.
	var raw []byte
	var result Result
	if workErr == nil {
		enabled, workErr = e.Store.Enabled(run)
		if !enabled && workErr == nil {
			workErr = context.Canceled
		}
	}
	if workErr == nil {
		var url, ua string
		url, ua, workErr = ticket.Claim()
		if workErr == nil {
			raw, workErr = e.Probe.Probe(run, url, ProbeOptions{UserAgent: ua, Timeout: e.ProbeTimeout})
		}
	}
	if workErr == nil {
		parser := e.Parser
		if parser == nil {
			parser = ResultParser{}
		}
		result, workErr = parser.Parse(raw)
	}
	// Never log/return raw JSON or the original process/provider error.
	save, closeSave := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer closeSave()
	var committed bool
	var saveErr error
	reason := "ready"
	if workErr == nil {
		committed, saveErr = e.Store.Succeed(save, *l, result)
	} else {
		failure := classifyFailure(workErr)
		reason = failure.Reason
		committed, saveErr = e.Store.Fail(save, *l, failure)
	}
	if workErr == nil && saveErr == nil && committed && e.AfterReady != nil {
		if err := e.AfterReady(save, l.AssetID); err != nil && e.Logger != nil {
			e.Logger.Warn("Native still enqueue deferred", "assetId", l.AssetID)
		}
	}
	if saveErr != nil {
		reason = "write_unavailable"
	} else if !committed {
		reason = "abandoned"
	}
	if e.Logger != nil {
		e.Logger.Info("Native probe attempt", "assetId", l.AssetID, "result", reason)
	}
	return nil
}
func classifyFailure(err error) Failure {
	var f *linksource.Failure
	if errors.As(err, &f) {
		out := Failure{f.Class, string(f.Reason), f.RetryAfter}
		if out.valid() {
			return out
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Failure{Class: linksource.Retryable, Reason: string(linksource.Timeout)}
	}
	if errors.Is(err, ffx.ErrUnavailable) {
		return Failure{Class: linksource.Manual, Reason: string(linksource.Unavailable)}
	}
	if errors.Is(err, ErrInvalid) {
		return Failure{Class: linksource.Permanent, Reason: string(InvalidResult)}
	}
	return Failure{Class: linksource.Retryable, Reason: string(ProcessFailed)}
}
