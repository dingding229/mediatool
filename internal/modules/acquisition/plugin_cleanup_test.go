package acquisition

import (
	"strings"
	"testing"
)

func TestCleanupProtectionMarkers(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	for _, tag := range []string{"keep", "protect", "no-cleanup", "plugin:another", "rule:another", "00000000-0000-0000-0000-000000000002"} {
		if !protectedCleanupTags(id+", "+tag, id) {
			t.Fatal(tag)
		}
	}
	if protectedCleanupTags(id+", normal-label", id) {
		t.Fatal("ordinary tag protected")
	}
	if !protectedCleanupTags(strings.ToUpper("KEEP"), id) {
		t.Fatal("case bypass")
	}
}
