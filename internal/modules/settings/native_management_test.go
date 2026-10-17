package settings

import (
	"strings"
	"testing"
)

func TestNativeManagementValidationMatchesSchema(t *testing.T) {
	for _, v := range []struct {
		name     string
		revision int64
		valid    bool
	}{{"Synthetic", 1, true}, {strings.Repeat("中", 100), 1, true}, {strings.Repeat("中", 101), 1, false}, {"", 1, false}, {" Synthetic ", 1, false}, {"a\nb", 1, false}, {"Synthetic", 0, false}} {
		if got := (NativeManagement{ServerName: v.name, Revision: v.revision}).Valid(); got != v.valid {
			t.Fatalf("name length %d revision %d: %v", len(v.name), v.revision, got)
		}
	}
}
