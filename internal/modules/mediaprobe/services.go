package mediaprobe

import (
	"context"
	"errors"
	"time"
)

var ErrNotImplemented = errors.New("native media preprocessing not implemented")

type Stream struct {
	Index                   int
	Type, Codec, Language   string
	Width, Height, Channels int
	Bitrate                 int64
	Default, Forced         bool
}
type Chapter struct {
	StartTicks int64
	Name       string
}
type ProbeResult struct {
	DurationTicks, Bitrate int64
	Format                 string
	Streams                []Stream
	Chapters               []Chapter
}
type Image struct {
	Bytes         []byte
	MIME          string
	Width, Height int
}
type Prober interface {
	Probe(context.Context, string) (ProbeResult, error)
}
type StillGrabber interface {
	Grab(context.Context, string, time.Duration) (Image, error)
}
type empty struct{}

func NewProber() Prober             { return empty{} }
func NewStillGrabber() StillGrabber { return empty{} }
func (empty) Probe(context.Context, string) (ProbeResult, error) {
	return ProbeResult{}, ErrNotImplemented
}
func (empty) Grab(context.Context, string, time.Duration) (Image, error) {
	return Image{}, ErrNotImplemented
}
