package subtitles

import (
	"bytes"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var timestamp = regexp.MustCompile(`(?m)(?:[0-9]{2}:)?[0-9]{2}:[0-9]{2}[,.][0-9]{3}\s*-->\s*(?:[0-9]{2}:)?[0-9]{2}:[0-9]{2}[,.][0-9]{3}`)

func Normalize(name, charset string, raw []byte) ([]byte, string, string, error) {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if len(raw) == 0 || len(raw) > MaxBytes || !strings.Contains("|srt|ass|ssa|vtt|", "|"+ext+"|") || len([]rune(name)) > 256 || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n/\\") {
		return nil, "", "", ErrInvalid
	}
	encodings := map[string]encoding.Encoding{"gb18030": simplifiedchinese.GB18030, "big5": traditionalchinese.Big5, "shift-jis": japanese.ShiftJIS, "windows-1252": charmap.Windows1252, "utf-16le": unicodeenc.UTF16(unicodeenc.LittleEndian, unicodeenc.IgnoreBOM), "utf-16be": unicodeenc.UTF16(unicodeenc.BigEndian, unicodeenc.IgnoreBOM), "utf-32le": utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM), "utf-32be": utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM)}
	charset = strings.ToLower(charset)
	if charset == "" || charset == "auto" {
		switch {
		case bytes.HasPrefix(raw, []byte{0xff, 0xfe, 0, 0}):
			charset = "utf-32le"
			raw = raw[4:]
		case bytes.HasPrefix(raw, []byte{0, 0, 0xfe, 0xff}):
			charset = "utf-32be"
			raw = raw[4:]
		case bytes.HasPrefix(raw, []byte{0xff, 0xfe}):
			charset = "utf-16le"
			raw = raw[2:]
		case bytes.HasPrefix(raw, []byte{0xfe, 0xff}):
			charset = "utf-16be"
			raw = raw[2:]
		case unicodeGuess(raw) != "":
			charset = unicodeGuess(raw)
		case utf8.Valid(raw):
			charset = "utf-8"
		default:
			charset = "gb18030"
			lang := Infer(name).Language
			if lang == "cht" || lang == "zh-tw" {
				charset = "big5"
			}
			if lang == "jpn" || lang == "ja" {
				charset = "shift-jis"
			}
			if lang == "eng" || lang == "en" {
				charset = "windows-1252"
			}
		}
	}
	var body []byte
	var err error
	if charset == "utf-8" {
		body = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
		if !utf8.Valid(body) {
			return nil, "", "", ErrInvalid
		}
	} else if e := encodings[charset]; e != nil {
		body, err = e.NewDecoder().Bytes(raw)
	} else {
		return nil, "", "", ErrInvalid
	}
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	if err != nil || len(body) == 0 || len(body) > MaxBytes || !utf8.Valid(body) || bytes.ContainsRune(body, '\ufffd') || bytes.IndexFunc(body, func(r rune) bool { return r < 32 && r != '\r' && r != '\n' && r != '\t' }) >= 0 {
		return nil, "", "", ErrInvalid
	}
	body = bytes.ReplaceAll(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	lower := strings.ToLower(string(body))
	valid := timestamp.Match(body)
	if ext == "vtt" {
		valid = valid && strings.HasPrefix(string(body), "WEBVTT")
	}
	if ext == "ass" || ext == "ssa" {
		valid = strings.Contains(lower, "[script info]") && strings.Contains(lower, "[events]")
	}
	if !valid {
		return nil, "", "", ErrInvalid
	}
	return body, ext, charset, nil
}
func ContentType(ext string) string {
	switch ext {
	case "srt":
		return "application/x-subrip; charset=utf-8"
	case "ass", "ssa":
		return "text/x-ssa; charset=utf-8"
	case "vtt":
		return "text/vtt; charset=utf-8"
	}
	return ""
}

// Subtitle headers/timestamps contain ASCII. Alternating zero bytes identify
// BOM-less UTF-16/32 conservatively; legacy encodings remain explicitly selectable.
func unicodeGuess(raw []byte) string {
	if len(raw) < 8 {
		return ""
	}
	sample := raw[:min(len(raw), 4096)]
	for _, width := range []int{4, 2} {
		if len(raw)%width != 0 {
			continue
		}
		for _, little := range []bool{true, false} {
			ascii := 0
			units := len(sample) / width
			for i := 0; i+width <= len(sample); i += width {
				position := width - 1
				if little {
					position = 0
				}
				valid := sample[i+position] > 0 && sample[i+position] < 128
				for j := 0; j < width; j++ {
					if j != position && sample[i+j] != 0 {
						valid = false
					}
				}
				if valid {
					ascii++
				}
			}
			if ascii*2 >= units {
				if width == 4 {
					if little {
						return "utf-32le"
					}
					return "utf-32be"
				}
				if little {
					return "utf-16le"
				}
				return "utf-16be"
			}
		}
	}
	return ""
}
