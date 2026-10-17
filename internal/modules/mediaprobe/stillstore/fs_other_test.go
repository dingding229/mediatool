//go:build !linux && !darwin

package stillstore

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "native-media", "stills")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
