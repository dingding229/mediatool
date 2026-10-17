package nativelibrary

import (
	"media-tool/internal/modules/embyapi/dto"
	"testing"
)

func TestCompleteUserDataPurePercentageAndStableKey(t *testing.T) {
	runtime := int64(6000000000)
	for _, x := range []struct {
		pos     int64
		played  bool
		runtime *int64
		want    float64
	}{{1200000000, false, &runtime, 20}, {7000000000, false, &runtime, 100}, {1200000000, false, nil, 0}, {0, true, nil, 100}} {
		d := CompleteUserData(dto.UserItemDataDto{PlaybackPositionTicks: &x.pos, Played: x.played}, "100035", x.runtime)
		if d.Key != "100035" || d.PlayCount == nil || *d.PlayCount != 0 || d.PlayedPercentage == nil || *d.PlayedPercentage != x.want || d.LastPlayedDate != nil {
			t.Fatal(d)
		}
	}
}
