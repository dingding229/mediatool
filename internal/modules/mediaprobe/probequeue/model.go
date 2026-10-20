// Package probequeue owns durable attempts and bounded background execution.
// Jobs only wake consumers; raw ffprobe JSON never crosses typed persistence.
package probequeue

import (
	"errors"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	"media-tool/internal/modules/mediaprobe/retry"
	"media-tool/internal/platform/requestbudget"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalid = errors.New("invalid probe attempt")
var ErrNotExposable = errors.New("probe asset not exposable")

type AttemptReason string

const (
	LeaseExpired  AttemptReason = "lease_expired"
	ProcessFailed AttemptReason = "process_failed"
	InvalidResult AttemptReason = "invalid_result"
)

// Result is the pure whitelist, not raw process output.
type Result = probeinfo.Result
type Snapshot struct {
	Result      Result
	Fingerprint string
	ProbedAt    time.Time
}
type Lease struct {
	AssetID, CloudFileID, Fingerprint, Token string
	Attempt                                  int
}
type Failure struct {
	Class      linksource.Class
	Reason     string
	RetryAfter time.Duration
}

func (f Failure) valid() bool {
	if f.Class != linksource.Permanent && f.Class != linksource.Retryable && f.Class != linksource.Manual {
		return false
	}
	switch f.Reason {
	case string(linksource.NotExposable), string(linksource.FileMissing), string(linksource.Mismatch), string(linksource.Authorization), string(linksource.Limited), string(linksource.Timeout), string(linksource.Unavailable), string(linksource.UnsafeTarget), string(linksource.UAMismatch), string(linksource.Used), string(LeaseExpired), string(ProcessFailed), string(InvalidResult):
		return true
	}
	return false
}
func validID(s string) bool { var id pgtype.UUID; return len(s) == 36 && id.Scan(s) == nil && id.Valid }
func validLease(l Lease) bool {
	return validID(l.AssetID) && validID(l.CloudFileID) && validID(l.Token) && len(l.Fingerprint) == 64 && l.Attempt > 0
}

// Suggested upstream intervals take priority; both paths cap at 24 hours.
func Backoff(attempt int, suggested time.Duration) time.Duration {
	return retry.Backoff(attempt, suggested)
}
func LeaseDuration(resolve, probe time.Duration) (time.Duration, error) {
	if resolve < 0 || resolve > requestbudget.MaxConfiguredResolve || probe < 0 || probe > 2*time.Minute {
		return 0, ErrInvalid
	}
	if probe == 0 {
		probe = 30 * time.Second
	}
	return requestbudget.EffectiveResolve(resolve) + probe + 15*time.Second, nil
}
