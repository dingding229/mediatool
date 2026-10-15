package plugins

import (
	"errors"
	"testing"
)

func TestFailedInitializationNeverAttemptsEnable(t *testing.T) {
	want := errors.New("init failed")
	enabled := false
	if err := initializeBeforeEnable(func() error { return want }, func() error { enabled = true; return nil }); !errors.Is(err, want) || enabled {
		t.Fatal("failed plugin became enabled", err, enabled)
	}
}
func TestInitializationEnableFailureIsReported(t *testing.T) {
	initialized := false
	want := errors.New("enable not confirmed")
	err := initializeBeforeEnable(func() error { initialized = true; return nil }, func() error {
		if !initialized {
			t.Fatal("enabled before initialization")
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
}
