// Package linksource fetches uncached, one-use links for trusted background
// preprocessing. It exposes no HTTP handler and changes no player UA policy.
package linksource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"media-tool/internal/modules/storage"
)

type Class string
type Reason string

const (
	Permanent Class = "permanent"
	Retryable Class = "retryable"
	Manual    Class = "manual"
)
const (
	NotExposable  Reason = "not_exposable"
	FileMissing   Reason = "file_missing"
	Mismatch      Reason = "strm_mismatch"
	Authorization Reason = "authorization"
	Limited       Reason = "rate_limited"
	Timeout       Reason = "timeout"
	Unavailable   Reason = "unavailable"
	UnsafeTarget  Reason = "unsafe_target"
	UAMismatch    Reason = "ua_mismatch"
	Used          Reason = "ticket_unavailable"
)

type Failure struct {
	Class      Class
	Reason     Reason
	RetryAfter time.Duration
}

func (f *Failure) Error() string { return "probe link: " + string(f.Class) + "/" + string(f.Reason) }
func fail(class Class, reason Reason, delay time.Duration) error {
	return &Failure{class, reason, delay}
}

type Evidence struct {
	AssetID, Path, PickCode, Output string
	Size, Revision                  int64
	Digest                          []byte
}
type Repository interface {
	Load(context.Context, string) (Evidence, error)
}
type Reader interface {
	Read(context.Context, string) ([]byte, error)
}
type Parser interface {
	Parse(context.Context, string) (storage.FileRef, error)
}
type TargetPolicy interface {
	Check(context.Context, string) error
}
type Config struct {
	UserAgent      string
	ResolveTimeout time.Duration
}
type Dependencies struct {
	Repository Repository
	Reader     Reader
	Parser     Parser
	Resolver   storage.PlaybackResolver
	Policy     TargetPolicy
}

// Only Claim returns URL/UA. All ordinary formatting and JSON are redacted.
type Ticket struct{ state *ticketState }
type ticketState struct {
	mu      sync.Mutex
	url, ua string
	expires time.Time
}

func (t *Ticket) Claim() (string, string, error) {
	if t == nil || t.state == nil {
		return "", "", fail(Permanent, Used, 0)
	}
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	if t.state.url == "" || time.Now().After(t.state.expires) {
		t.state.url = ""
		t.state.ua = ""
		return "", "", fail(Permanent, Used, 0)
	}
	link, ua := t.state.url, t.state.ua
	t.state.url = ""
	t.state.ua = ""
	return link, ua, nil
}
func (Ticket) Format(s fmt.State, _ rune) {
	if _, err := io.WriteString(s, "probe link [redacted]"); err != nil {
		return
	}
}
func (Ticket) MarshalJSON() ([]byte, error) { return json.Marshal(struct{}{}) }

type probeKey struct{}

func trustedProbe(ctx context.Context) bool { value, _ := ctx.Value(probeKey{}).(bool); return value }
