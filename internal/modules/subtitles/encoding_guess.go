package subtitles

import (
	"bytes"
	"errors"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

func legacyCharset(name string, raw []byte) string {
	d := FilenameLanguage(name)
	// Script/language markers do not establish a legacy encoding: .cht can
	// contain GB18030 and .chs can contain Big5. Compare the decoded text.
	if d.Language == "eng" {
		return "windows-1252"
	}
	sample := raw
	if len(sample) > 65536 {
		sample = sample[:65536]
	}
	best, bestScore := "gb18030", -1
	for _, candidate := range []struct {
		name  string
		codec encoding.Encoding
	}{{"gb18030", simplifiedchinese.GB18030}, {"big5", traditionalchinese.Big5}, {"shift-jis", japanese.ShiftJIS}} {
		// A bounded sample may end halfway through a multibyte character.
		// Do not synthesize a replacement rune at that artificial boundary.
		decoded := make([]byte, len(sample)*3+3)
		n, _, e := candidate.codec.NewDecoder().Transform(decoded, sample, len(sample) == len(raw))
		decoded = decoded[:n]
		if e != nil && !errors.Is(e, transform.ErrShortSrc) || bytes.ContainsRune(decoded, '\ufffd') {
			continue
		}
		c := countLanguage(string(decoded))
		// Random GBK/Big5 bytes often decode as half-width kana in Shift-JIS;
		// those alone are weak encoding evidence, unlike full kana phrases.
		score := c.common*8 + c.simple*3 + c.traditional*3 + (c.kana-c.halfKana)*6 + c.halfKana + c.hangul*6 - c.latin
		if score > bestScore {
			best, bestScore = candidate.name, score
		}
	}
	return best
}
