package textkey

import "testing"

func TestTitlePreservesUnicodeAndDoesNotMatchIdentity(t *testing.T) {
	if Title("幽灵公主 - Movie.2026") != "幽灵公主movie2026" || Title("A_B-C") != "abc" || Title("!!!") != "" {
		t.Fatal("title normalization drift")
	}
}
