// Package stillrunner is the background-only episode capture executor.
package stillrunner

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"time"

	"media-tool/internal/modules/mediaprobe/stillqueue"
)

var ErrImage = errors.New("invalid still frame")

type Ticket interface {
	Claim() (string, string, error)
}
type Links interface {
	Reserve(context.Context) (context.Context, func(), error)
	Fetch(context.Context, string) (Ticket, error)
}
type FrameOptions struct {
	UserAgent              string
	Timeout                time.Duration
	Width, Height, Quality int
}
type Frames interface {
	Reserve(context.Context) (context.Context, func(), error)
	Still(context.Context, string, time.Duration, FrameOptions) ([]byte, error)
}
type FrameStats struct{ Mean, Variance float64 }

func Analyze(raw []byte) (FrameStats, error) {
	if len(raw) == 0 || int64(len(raw)) > stillqueue.MaxImageBytes {
		return FrameStats{}, ErrImage
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1280 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 16<<20 {
		return FrameStats{}, ErrImage
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return FrameStats{}, ErrImage
	}
	var out FrameStats
	var n float64
	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			v := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 257
			n++
			delta := v - out.Mean
			out.Mean += delta / n
			out.Variance += delta * (v - out.Mean)
		}
	}
	out.Variance /= n
	return out, nil
}
func (s FrameStats) Blank() bool { return s.Mean <= 8 || s.Mean >= 247 }
func offset(ticks int64, percent int) (time.Duration, error) {
	if ticks < 100000 || percent < 10 || percent > 20 {
		return 0, ErrImage
	}
	part := ticks/100*int64(percent) + ticks%100*int64(percent)/100
	if part < 1 || part > int64(24*time.Hour/100) {
		return 0, ErrImage
	}
	return time.Duration(part) * 100, nil
}

// Caller has already reserved the still lane AND the first link permit before
// claiming a durable lease. Each subsequent frame acquires a NEW link permit,
// one-use ticket and independent resolve/frame timeout (never reuses a URL).
func Capture(ctx context.Context, l stillqueue.Lease, links Links, frames Frames, c stillqueue.Config) ([]byte, error) {
	var err error
	c, err = c.Normalize()
	if err != nil {
		return nil, ErrImage
	}
	var best []byte
	var variance float64 = -1
	for i, p := range []int{15, 12, 18} {
		at, err := offset(l.DurationTicks, p)
		if err != nil {
			return nil, err
		}
		resolve, stop := context.WithTimeout(ctx, c.ResolveTimeout)
		reserved := resolve
		release := func() {}
		if i > 0 {
			reserved, release, err = links.Reserve(resolve)
		}
		var ticket Ticket
		if err == nil {
			ticket, err = links.Fetch(reserved, l.AssetID)
		}
		release()
		stop()
		if err != nil {
			return nil, err
		}
		if ticket == nil {
			return nil, ErrImage
		}
		url, ua, err := ticket.Claim()
		if err != nil {
			return nil, err
		}
		raw, err := frames.Still(ctx, url, at, FrameOptions{UserAgent: ua, Timeout: c.FrameTimeout, Width: 1280, Height: 1080, Quality: 3})
		if err != nil {
			return nil, err
		}
		stats, err := Analyze(raw)
		if err != nil {
			return nil, err
		}
		if !stats.Blank() {
			return raw, nil
		}
		if stats.Variance > variance {
			best, variance = raw, stats.Variance
		}
	}
	return best, nil
}
