package stillrunner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
)

type Attempts interface {
	Enabled(context.Context) (bool, error)
	Claim(context.Context, int64) (*stillqueue.Lease, error)
	Succeed(context.Context, stillqueue.Lease, stillqueue.Image) (bool, error)
	Fail(context.Context, stillqueue.Lease, stillqueue.Failure) (bool, error)
}
type Publisher interface {
	Publish(context.Context, stillqueue.Lease, []byte) (stillqueue.Image, error)
}
type Engine struct {
	Store  Attempts
	Links  Links
	Frames Frames
	Files  Publisher
	Config stillqueue.Config
	Logger *slog.Logger
}

// jobs only wakes; even execution/persistence failure returns success to jobs.
// Recovery and retry timing belong exclusively to media_stills.
func (e Engine) Process(parent context.Context, j jobs.Job) error {
	var in struct {
		ItemID int64 `json:"itemId"`
	}
	if json.Unmarshal(j.Payload, &in) != nil || in.ItemID < 100000 || e.Store == nil || e.Links == nil || e.Frames == nil || e.Files == nil {
		return nil
	}
	c, err := e.Config.Normalize()
	if err != nil {
		return nil
	}
	on, err := e.Store.Enabled(parent)
	if err != nil || !on {
		return nil
	}
	wait, stop := context.WithTimeout(parent, 2*time.Minute+c.Lease)
	defer stop()
	reserved, releaseFrame, err := e.Frames.Reserve(wait)
	if err != nil {
		return nil
	}
	defer releaseFrame()
	reserved, releaseLink, err := e.Links.Reserve(reserved)
	if err != nil {
		return nil
	}
	defer releaseLink()
	l, err := e.Store.Claim(reserved, in.ItemID)
	if err != nil || l == nil {
		return nil
	}
	run, cancel := context.WithTimeout(reserved, c.Lease-5*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-ticker.C:
				on, err := e.Store.Enabled(run)
				if err != nil || !on {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	on, workErr := e.Store.Enabled(run)
	if !on && workErr == nil {
		workErr = context.Canceled
	}
	var raw []byte
	var image stillqueue.Image
	if workErr == nil {
		raw, workErr = Capture(run, *l, e.Links, e.Frames, c)
	}
	if workErr == nil {
		image, workErr = e.Files.Publish(run, *l, raw)
	}
	save, closeSave := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer closeSave()
	var committed bool
	var saveErr error
	reason := "ready"
	if workErr == nil {
		committed, saveErr = e.Store.Succeed(save, *l, image)
	} else {
		f := classify(workErr)
		reason = f.Reason
		committed, saveErr = e.Store.Fail(save, *l, f)
	}
	if saveErr != nil {
		reason = "write_unavailable"
	} else if !committed {
		reason = "abandoned"
	}
	if e.Logger != nil {
		e.Logger.Info("Native still attempt", "assetId", l.AssetID, "result", reason)
	}
	return nil
}
func classify(err error) stillqueue.Failure {
	var link *linksource.Failure
	if errors.As(err, &link) {
		return stillqueue.Failure{Class: link.Class, Reason: string(link.Reason), RetryAfter: link.RetryAfter}
	}
	var retry *stillstore.RetryError
	if errors.As(err, &retry) {
		return stillqueue.Failure{Class: linksource.Retryable, Reason: string(retry.Reason), RetryAfter: retry.After}
	}
	if errors.Is(err, ErrImage) || errors.Is(err, stillstore.ErrInvalid) || errors.Is(err, stillstore.ErrConflict) {
		return stillqueue.Failure{Class: linksource.Permanent, Reason: string(stillqueue.InvalidImage)}
	}
	if errors.Is(err, ffx.ErrUnavailable) {
		return stillqueue.Failure{Class: linksource.Manual, Reason: string(linksource.Unavailable)}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return stillqueue.Failure{Class: linksource.Retryable, Reason: string(linksource.Timeout)}
	}
	return stillqueue.Failure{Class: linksource.Retryable, Reason: string(stillqueue.ProcessFailed)}
}
