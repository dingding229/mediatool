package mediaprobe

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPreprocessingStubsDoNotReadLinksOrProduceMedia(t *testing.T) {
	for i := 0; i < 2; i++ {
		if result, err := NewProber().Probe(context.Background(), "not-a-link"); !errors.Is(err, ErrNotImplemented) || !reflect.DeepEqual(result, ProbeResult{}) {
			t.Fatal(result, err)
		}
		if image, err := NewStillGrabber().Grab(context.Background(), "not-a-link", time.Second); !errors.Is(err, ErrNotImplemented) || !reflect.DeepEqual(image, Image{}) {
			t.Fatal(image, err)
		}
	}
}
