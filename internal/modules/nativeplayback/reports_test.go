package nativeplayback

import "testing"

func TestViewingProgressThresholds(t *testing.T) {
	const minute = int64(600000000)
	for _, v := range []struct {
		name                       string
		position, duration, resume int64
		played                     bool
	}{
		{"under5", minute / 3, 10 * minute, 0, false}, {"exact5", minute / 2, 10 * minute, minute / 2, false},
		{"middle", 2 * minute, 10 * minute, 2 * minute, false}, {"exact90", 9 * minute, 10 * minute, 9 * minute, false},
		{"over90", 95 * minute / 10, 10 * minute, 0, true}, {"short", minute, 4 * minute, 0, false},
		{"shortCompleted", 4 * minute, 4 * minute, 0, true}, {"unknown", minute, 0, minute, false},
	} {
		t.Run(v.name, func(t *testing.T) {
			p, played := ViewingProgress(v.position, v.duration)
			if p != v.resume || played != v.played {
				t.Fatal(p, played)
			}
		})
	}
}
