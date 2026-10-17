package stillrunner

import (
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
)

type fakeAttempts struct {
	mu                    sync.Mutex
	on                    bool
	claims, saved, failed int
	failure               stillqueue.Failure
	beforeClaim           func()
}

func (f *fakeAttempts) Enabled(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on, nil
}
func (f *fakeAttempts) Claim(context.Context, int64) (*stillqueue.Lease, error) {
	if f.beforeClaim != nil {
		f.beforeClaim()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims++
	return &stillqueue.Lease{ItemID: 100001, AssetID: "synthetic", DurationTicks: 30_000_000}, nil
}
func (f *fakeAttempts) Succeed(context.Context, stillqueue.Lease, stillqueue.Image) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved++
	return true, nil
}
func (f *fakeAttempts) Fail(_ context.Context, _ stillqueue.Lease, failure stillqueue.Failure) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed++
	f.failure = failure
	return true, nil
}

type fakePublisher struct {
	err   error
	calls int
}

func (f *fakePublisher) Publish(context.Context, stillqueue.Lease, []byte) (stillqueue.Image, error) {
	f.calls++
	return stillqueue.Image{}, f.err
}

type waitingFrames struct {
	fakeFrames
	entered, release chan struct{}
}

func (f *waitingFrames) Reserve(ctx context.Context) (context.Context, func(), error) {
	close(f.entered)
	select {
	case <-f.release:
		return ctx, func() {}, nil
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	}
}
func engineJob(t *testing.T) jobs.Job {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"itemId": 100001})
	if err != nil {
		t.Fatal(err)
	}
	return jobs.Job{Payload: raw}
}
func TestEngineReservesBeforeLeaseAndNeverRetriesJobs(t *testing.T) {
	store := &fakeAttempts{on: true}
	frames := &waitingFrames{fakeFrames: fakeFrames{raw: [][]byte{jpegFrame(t, color.Gray{Y: 100})}}, entered: make(chan struct{}), release: make(chan struct{})}
	files := &fakePublisher{err: &stillstore.RetryError{Reason: stillqueue.StorageFull, After: 3 * time.Minute}}
	links := &fakeLinks{url: "synthetic"}
	engine := Engine{Store: store, Links: links, Frames: frames, Files: files}
	done := make(chan error, 1)
	job := engineJob(t)
	go func() { done <- engine.Process(t.Context(), job) }()
	<-frames.entered
	store.mu.Lock()
	claims := store.claims
	store.mu.Unlock()
	if claims != 0 {
		t.Fatal("lease occupied while waiting")
	}
	close(frames.release)
	if err := <-done; err != nil {
		t.Fatal("jobs generic retry triggered", err)
	}
	if store.saved != 0 || store.failed != 1 || store.failure.Class != linksource.Retryable || store.failure.RetryAfter != 3*time.Minute {
		t.Fatal(store)
	}
	store.on = false
	if err := engine.Process(t.Context(), engineJob(t)); err != nil || store.claims != 1 {
		t.Fatal("disabled engine executed", err)
	}
}
func TestFailureClassificationAndStartupBudget(t *testing.T) {
	for _, tt := range []struct {
		err   error
		class linksource.Class
	}{{context.Canceled, linksource.Retryable}, {ErrImage, linksource.Permanent}, {&linksource.Failure{Class: linksource.Manual, Reason: linksource.Authorization}, linksource.Manual}, {errors.New("https://synthetic.invalid/?token=NEVER_LOG"), linksource.Retryable}} {
		if got := classify(tt.err); got.Class != tt.class {
			t.Fatal(got)
		}
	}
	store := &fakeAttempts{on: true}
	e := Engine{Store: store, Links: &fakeLinks{}, Frames: &fakeFrames{}, Files: &fakePublisher{}, Config: stillqueue.Config{ResolveTimeout: time.Minute}}
	if err := e.Process(t.Context(), engineJob(t)); err != nil || store.claims != 0 {
		t.Fatal("bad budget accepted")
	}
}

type cancelFrames struct {
	fakeFrames
	entered chan struct{}
}

func (f *cancelFrames) Still(ctx context.Context, _ string, _ time.Duration, _ FrameOptions) ([]byte, error) {
	close(f.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestEngineCancelsWhenSwitchTurnsOffAndRedactsErrors(t *testing.T) {
	store := &fakeAttempts{on: true}
	frames := &cancelFrames{entered: make(chan struct{})}
	files := &fakePublisher{}
	engine := Engine{Store: store, Links: &fakeLinks{url: "synthetic"}, Frames: frames, Files: files}
	job := engineJob(t)
	done := make(chan error, 1)
	go func() { done <- engine.Process(t.Context(), job) }()
	select {
	case <-frames.entered:
	case <-time.After(time.Second):
		t.Fatal("capture did not start")
	}
	store.mu.Lock()
	store.on = false
	store.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("switch did not cancel capture")
	}
	if store.saved != 0 || files.calls != 0 || store.failure.Class != linksource.Retryable {
		t.Fatal("disabled capture published")
	}
}
