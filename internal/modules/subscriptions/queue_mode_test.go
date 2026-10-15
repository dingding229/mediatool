package subscriptions

import (
	"errors"
	"testing"
)

func TestRunModeConflictDoesNotReusePreviewForExecution(t *testing.T) {
	for _, tc := range []struct {
		existing, requested bool
		want                bool
	}{{true, false, true}, {false, true, true}, {true, true, false}, {false, false, false}} {
		err := runModeConflict(tc.existing, tc.requested)
		if (err != nil) != tc.want || tc.want && !errors.Is(err, ErrRunModeConflict) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}
