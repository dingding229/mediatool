// Package stillqueue owns episode-still attempts, separate from probequeue.
// It performs only bounded database operations; no worker or file I/O runs here.
package stillqueue

import (
	"errors"
	"media-tool/internal/modules/mediaprobe/retry"
	"regexp"
	"time"

	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/platform/requestbudget"
)

var ErrInvalid = errors.New("invalid still attempt")
var ErrDisabled = errors.New("native still processing disabled")
var ErrNotEligible = errors.New("episode not eligible for still processing")

const DefaultMaxBytes int64 = 2 << 30
const MaxImageBytes int64 = 4 << 20
const EvictionCooldown = 24 * time.Hour

const MaxLease = 5 * time.Minute
const FinalizeBudget = 30 * time.Second

type Config struct {
	ResolveTimeout, FrameTimeout, Lease, GCGrace time.Duration
	MaxBytes                                     int64
}

// Normalize is the startup validation contract. T25-C must construct through New
// before starting workers; no runtime worker is registered in T25-A.
func (c Config) Normalize() (Config, error) {
	if c.ResolveTimeout == 0 {
		c.ResolveTimeout = requestbudget.DefaultResolve
	}
	if c.FrameTimeout == 0 {
		c.FrameTimeout = 45 * time.Second
	}
	if c.Lease == 0 {
		c.Lease = MaxLease
	}
	if c.GCGrace == 0 {
		c.GCGrace = 15 * time.Minute
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxBytes < 1 || c.MaxBytes > 1<<40 {
		return Config{}, ErrInvalid
	}
	if c.Lease > MaxLease {
		return Config{}, ErrInvalid
	}
	c.Lease = c.Lease.Truncate(time.Millisecond)
	if c.ResolveTimeout <= 0 || c.ResolveTimeout > requestbudget.MaxConfiguredResolve || c.FrameTimeout <= 0 || c.FrameTimeout > 2*time.Minute || c.Lease > MaxLease || c.Lease < 3*(c.ResolveTimeout+c.FrameTimeout)+FinalizeBudget || c.GCGrace <= 2*MaxLease {
		return Config{}, ErrInvalid
	}
	return c, nil
}

type Lease struct {
	ItemID                      int64
	AssetID, Fingerprint, Token string
	DurationTicks               int64
	Attempt                     int
	Until                       time.Time
}
type Image struct {
	RelativePath, Digest string
	Width, Height        int
	SizeBytes            int64
}
type Snapshot struct {
	Image
	ItemID                    int64
	AssetID, Fingerprint      string
	PublishedAt, LastAccessAt time.Time
}
type Failure struct {
	Class      linksource.Class
	Reason     string
	RetryAfter time.Duration
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(s string) bool { return uuidPattern.MatchString(s) }

func validLease(l Lease) bool {
	return l.ItemID >= 100000 && validID(l.AssetID) && validID(l.Token) && hex64.MatchString(l.Fingerprint) && l.Attempt > 0
}
func ImagePath(asset, fingerprint string) (string, error) {
	if !validID(asset) || !hex64.MatchString(fingerprint) {
		return "", ErrInvalid
	}
	return asset[:2] + "/" + asset + "-" + fingerprint[:16] + ".jpg", nil
}
func (f Failure) valid() bool {
	if f.Class != linksource.Permanent && f.Class != linksource.Retryable && f.Class != linksource.Manual {
		return false
	}
	switch f.Reason {
	case string(linksource.NotExposable), string(linksource.FileMissing), string(linksource.Mismatch), string(linksource.Authorization), string(linksource.Limited), string(linksource.Timeout), string(linksource.Unavailable), string(linksource.UnsafeTarget), string(linksource.UAMismatch), string(linksource.Used), string(LeaseExpired), string(ProcessFailed), string(InvalidImage), string(TMDBStill), string(SourceUnavailable), string(StorageUnavailable), string(StorageFull):
		return true
	}
	return false
}
func Backoff(attempt int, suggested time.Duration) time.Duration {
	return retry.Backoff(attempt, suggested)
}
