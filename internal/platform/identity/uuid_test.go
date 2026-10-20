package identity

import (
	"strings"
	"testing"
)

func TestCanonicalUUID(t *testing.T) {
	good := "01234567-89ab-cdef-0123-456789abcdef"
	for _, v := range []string{good, "00000000-0000-0000-0000-000000000000"} {
		if !UUID(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"", strings.ToUpper(good), strings.Replace(good, "-", "/", 1), good + "x", "01234567-89ab-cdef-0123-456789abcde中"} {
		if UUID(v) {
			t.Fatal(v)
		}
	}
}
