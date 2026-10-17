package stillqueue

import (
	"math"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediaprobe/linksource"
)

func TestConfigurationBounds(t *testing.T) {
	c, err := (Config{}).Normalize()
	if err != nil || c.ResolveTimeout != 30*time.Second || c.FrameTimeout != 45*time.Second || c.Lease != 5*time.Minute || c.GCGrace != 15*time.Minute || c.MaxBytes != DefaultMaxBytes {
		t.Fatal(c, err)
	}
	// Three independent links/frames + bounded finalize time must ALL fit.
	for _, bad := range []Config{
		{MaxBytes: -1}, {MaxBytes: 1<<40 + 1}, {ResolveTimeout: -1}, {FrameTimeout: -1}, {ResolveTimeout: 61 * time.Second}, {FrameTimeout: 121 * time.Second},
		{Lease: MaxLease + time.Nanosecond}, {Lease: 254 * time.Second}, {FrameTimeout: 70 * time.Second},
		{ResolveTimeout: 60 * time.Second, FrameTimeout: 45 * time.Second},
		{GCGrace: 2 * MaxLease}, {GCGrace: -1}, {Lease: time.Duration(math.MaxInt64)},
		{ResolveTimeout: time.Duration(math.MaxInt64)}, {FrameTimeout: time.Duration(math.MaxInt64)},
	} {
		if _, err := bad.Normalize(); err == nil {
			t.Fatal("unreasonable startup budget accepted", bad)
		}
	}
	for _, good := range []Config{{Lease: 255 * time.Second}, {ResolveTimeout: time.Minute, FrameTimeout: 30 * time.Second}, {GCGrace: 11 * time.Minute}} {
		if _, err := good.Normalize(); err != nil {
			t.Fatal(good, err)
		}
	}
	if _, err := New(nil, Config{}); err != ErrInvalid {
		t.Fatal("nil database")
	}
	if _, err := New(nil, Config{Lease: MaxLease + 1}); err != ErrInvalid {
		t.Fatal("constructor skipped config validation")
	}
}
func TestDeterministicPathAndBackoff(t *testing.T) {
	id := "ab111111-1111-4111-8111-111111111111"
	fp := strings.Repeat("a", 64)
	path, err := ImagePath(id, fp)
	if err != nil || path != "ab/"+id+"-"+fp[:16]+".jpg" {
		t.Fatal(path, err)
	}
	for _, tt := range [][2]string{{"../bad", fp}, {"ab111111/1111-4111-8111-111111111111", fp}, {strings.ToUpper(id), fp}, {id, strings.Repeat("A", 64)}, {id, "https://synthetic.invalid/?secret=NO"}, {"", fp}} {
		if _, err := ImagePath(tt[0], tt[1]); err == nil {
			t.Fatal("unsafe path accepted")
		}
	}
	for i, want := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		if Backoff(i+1, 0) != want {
			t.Fatal(i)
		}
	}
	if Backoff(math.MinInt, 0) != time.Minute || Backoff(1, 3*time.Second) != 3*time.Second || Backoff(100, 48*time.Hour) != 24*time.Hour {
		t.Fatal("suggested retry priority/bounds")
	}
	for _, class := range []linksource.Class{linksource.Permanent, linksource.Retryable, linksource.Manual} {
		for _, reason := range []string{string(LeaseExpired), string(TMDBStill), string(SourceUnavailable), string(InvalidImage), string(StorageFull), string(linksource.Limited)} {
			if !(Failure{Class: class, Reason: reason}).valid() {
				t.Fatal(class, reason)
			}
		}
	}
	for _, bad := range []Failure{{}, {Class: "unknown", Reason: string(LeaseExpired)}, {Class: linksource.Manual, Reason: "https://synthetic.invalid/?secret=NO"}} {
		if bad.valid() {
			t.Fatal("free error text accepted")
		}
	}
}
func TestEmptyRepositoryRejectsBeforeIO(t *testing.T) {
	var r Repository
	if _, err := r.Enabled(t.Context()); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Enqueue(t.Context(), 0); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Retry(t.Context(), 0); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Claim(t.Context(), 0); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Succeed(t.Context(), Lease{}, Image{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Fail(t.Context(), Lease{}, Failure{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.RecoverExpired(t.Context()); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Usage(t.Context()); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Admit(t.Context(), Lease{}, 0); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.EvictSnapshot(t.Context(), Snapshot{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.TouchBatch(t.Context(), nil); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.RequeueEvicted(t.Context(), 0); err != ErrInvalid {
		t.Fatal(err)
	}
	if (Failure{Class: linksource.Permanent, Reason: string(CacheEvicted)}).valid() {
		t.Fatal("cache eviction is not a worker failure")
	}
	if _, err := r.Current(t.Context(), 0); err != ErrInvalid {
		t.Fatal(err)
	}
}
