package metadata

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// EpisodeTag uses an explicit chain for sparse coverage. A range is permitted
// only if every intermediate number was actually declared. The parser round
// trip is an internal assertion, not a claim about all Emby client versions.
func EpisodeTag(season int, numbers []int) (string, error) {
	if season < 0 || season > 999 || len(numbers) == 0 || len(numbers) > 100 {
		return "", errors.New("invalid episode coverage")
	}
	ordered := append([]int(nil), numbers...)
	sort.Ints(ordered)
	contiguous := true
	for i, n := range ordered {
		if n < 1 || n > 9999 || i > 0 && n == ordered[i-1] {
			return "", errors.New("invalid or duplicate episode")
		}
		if i > 0 && n != ordered[i-1]+1 {
			contiguous = false
		}
	}
	value := fmt.Sprintf("S%02dE%02d", season, ordered[0])
	if len(ordered) > 1 {
		if contiguous {
			value += fmt.Sprintf("-E%02d", ordered[len(ordered)-1])
		} else {
			var b strings.Builder
			b.WriteString(value)
			for _, n := range ordered[1:] {
				fmt.Fprintf(&b, "E%02d", n)
			}
			value = b.String()
		}
	}
	return value, nil
}

func NormalizeEpisodes(season int, numbers []int) ([]int, error) {
	if _, err := EpisodeTag(season, numbers); err != nil {
		return nil, err
	}
	ordered := append([]int(nil), numbers...)
	sort.Ints(ordered)
	return ordered, nil
}
