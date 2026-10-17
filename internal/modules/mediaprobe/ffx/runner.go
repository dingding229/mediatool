// Package ffx is the background-only ffprobe/ffmpeg process boundary. It never
// accepts caller-supplied argv or exposes media delivery/transcoding endpoints.
package ffx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid media process configuration or input")
	ErrUnavailable = errors.New("media process executable unavailable")
	ErrOutputLimit = errors.New("media process stdout limit exceeded")
	ErrProcess     = errors.New("media process failed")
)

type Config struct {
	FFprobePath, FFmpegPath    string
	ProbeTimeout, StillTimeout time.Duration
	MaxOutputBytes             int
	AllowLocalFiles            bool // Tests only; zero-value production configuration rejects local paths.
}
type Options struct {
	UserAgent              string
	Timeout                time.Duration
	ProbeSize              int64
	AnalyzeDuration        time.Duration
	Width, Height, Quality int // Still JPEG only.
}
type Runner struct{ config Config }

func New(c Config) (*Runner, error) {
	if c.ProbeTimeout == 0 {
		c.ProbeTimeout = 30 * time.Second
	}
	if c.StillTimeout == 0 {
		c.StillTimeout = 45 * time.Second
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = 8 << 20
	}
	if !validTimeout(c.ProbeTimeout) || !validTimeout(c.StillTimeout) || c.MaxOutputBytes < 1 || c.MaxOutputBytes > 16<<20 || !validBinary(c.FFprobePath) || !validBinary(c.FFmpegPath) {
		return nil, ErrInvalid
	}
	return &Runner{config: c}, nil
}
func Probe(ctx context.Context, link string, opts Options) ([]byte, error) {
	r, err := New(Config{})
	if err != nil {
		return nil, err
	}
	return r.Probe(ctx, link, opts)
}
func Still(ctx context.Context, link string, offset time.Duration, opts Options) ([]byte, error) {
	r, err := New(Config{})
	if err != nil {
		return nil, err
	}
	return r.Still(ctx, link, offset, opts)
}
func (r *Runner) Probe(ctx context.Context, link string, opts Options) ([]byte, error) {
	return r.run(ctx, link, 0, opts, false)
}
func (r *Runner) Still(ctx context.Context, link string, offset time.Duration, opts Options) ([]byte, error) {
	return r.run(ctx, link, offset, opts, true)
}
func (r *Runner) run(parent context.Context, link string, offset time.Duration, opts Options, still bool) ([]byte, error) {
	if r == nil || parent == nil {
		return nil, ErrInvalid
	}
	input, args, timeout, err := r.arguments(link, offset, opts, still)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	release, err := acquireSlot(ctx, still)
	if err != nil {
		return nil, err
	}
	defer release()
	name, path := "ffprobe", r.config.FFprobePath
	if still {
		name, path = "ffmpeg", r.config.FFmpegPath
	}
	if path == "" {
		path = name
	}
	binary, err := exec.LookPath(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	if err := configureGroup(cmd); err != nil {
		return nil, ErrUnavailable
	}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error { return stopGroup(cmd, true) }
	output := &limitedOutput{limit: r.config.MaxOutputBytes, stop: func() error { return stopGroup(cmd, false) }}
	stderr := &tailOutput{}
	cmd.Stdout, cmd.Stderr = output, stderr
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	waitErr := cmd.Wait()               // Always reaps our child, including cancellation and overflow.
	cleanupErr := stopGroup(cmd, false) // Also kill descendants that outlived a successful parent.
	kind := ErrProcess
	switch {
	case output.exceeded():
		kind = ErrOutputLimit
	case ctx.Err() != nil:
		kind = ctx.Err()
	case waitErr == nil && cleanupErr == nil:
		if data := output.bytes(); len(data) > 0 {
			return data, nil
		}
		// ffmpeg can exit 0 without a frame when offset is past EOF. Never mark an
		// empty probe/still successful; downstream tasks must be able to retry it.
	}
	return nil, &ProcessError{Operation: name, Target: input.target, Stderr: stderr.safe(input), cause: kind}
}

// ProcessError never wraps exec errors/argv. Only sanitized diagnostics escape.
type ProcessError struct {
	Operation, Target, Stderr string `json:"-"`
	cause                     error
}

func (e *ProcessError) Error() string {
	return fmt.Sprintf("%s: %v (%s)", e.Operation, e.cause, e.Target)
}
func (e *ProcessError) Unwrap() error { return e.cause }
