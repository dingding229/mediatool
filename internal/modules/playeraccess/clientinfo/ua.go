package clientinfo

import "unicode/utf8"

const MaxUABytes = 4096

func ValidUA(ua string) bool {
	if len(ua) > MaxUABytes || !utf8.ValidString(ua) {
		return false
	}
	for _, c := range ua {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
