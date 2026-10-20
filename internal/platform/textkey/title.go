// Package textkey contains identity-neutral normalization; matching media still
// requires the caller's complete title/year/type/episode evidence.
package textkey

import (
	"strings"
	"unicode"
)

func Title(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
