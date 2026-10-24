package subtitles

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"media-tool/internal/platform/config"
)

const sample = "1\n00:00:01,000 --> 00:00:02,000\n字幕文本\n"

func TestNormalizeEncodingsAndInference(t *testing.T) {
	cases := []struct {
		name, text, want string
		enc              encoding.Encoding
	}{
		{"film.chs.default.forced.srt", sample, "gb18030", simplifiedchinese.GB18030},
		{"film.cht.srt", "1\n00:00:01,000 --> 00:00:02,000\n繁體字幕\n", "big5", traditionalchinese.Big5},
		{"film.jpn.srt", "1\n00:00:01,000 --> 00:00:02,000\n字幕テキスト\n", "shift-jis", japanese.ShiftJIS},
		{"film.eng.srt", "1\n00:00:01,000 --> 00:00:02,000\nCafé\n", "windows-1252", charmap.Windows1252},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			raw, e := c.enc.NewEncoder().Bytes([]byte(c.text))
			if e != nil {
				t.Fatal(e)
			}
			body, ext, detected, e := Normalize(c.name, "auto", raw)
			if e != nil || string(body) != c.text || ext != "srt" || detected != c.want {
				t.Fatal(string(body), ext, detected, e)
			}
		})
	}
	for _, c := range []struct {
		name string
		enc  encoding.Encoding
	}{
		{"utf-16le", unicodeenc.UTF16(unicodeenc.LittleEndian, unicodeenc.UseBOM)},
		{"utf-16be", unicodeenc.UTF16(unicodeenc.BigEndian, unicodeenc.UseBOM)},
		{"utf-32le", utf32.UTF32(utf32.LittleEndian, utf32.UseBOM)},
		{"utf-32be", utf32.UTF32(utf32.BigEndian, utf32.UseBOM)},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw, e := c.enc.NewEncoder().Bytes([]byte(sample))
			if e != nil {
				t.Fatal(e)
			}
			for _, bom := range []bool{true, false} {
				data := raw
				if !bom {
					n := 2
					if c.name[:6] == "utf-32" {
						n = 4
					}
					data = data[n:]
				}
				body, _, charset, e := Normalize("film.srt", "auto", data)
				if e != nil || charset != c.name || string(body) != sample {
					t.Fatal(bom, charset, e)
				}
			}
		})
	}
	for _, data := range [][]byte{[]byte(sample), append([]byte{0xef, 0xbb, 0xbf}, []byte(sample)...), []byte("1\r\n00:00:01,000 --> 00:00:02,000\r\n字幕文本\r\n")} {
		body, _, _, e := Normalize("film.srt", "auto", data)
		if e != nil || string(body) != sample {
			t.Fatal(e, string(body))
		}
	}
	m := Infer("film.chs.default.forced.srt")
	if m.Language != "chs" || !m.Default || !m.Forced {
		t.Fatal(m)
	}
	if Root(config.Config{}) != "/data/native-media/subtitles" || Root(config.Config{NativeStillsRoot: "/data/private/stills"}) != "/data/private/subtitles" {
		t.Fatal("subtitle persistence root")
	}
}
func TestNormalizeLimitsFormatsAndMetadata(t *testing.T) {
	for _, c := range []struct {
		name, charset string
		body          []byte
	}{
		{"film.exe", "auto", []byte(sample)}, {"film.srt", "auto", nil}, {"film.srt", "auto", bytes.Repeat([]byte("a"), MaxBytes+1)},
		{"film.srt", "auto", []byte("<script>alert(1)</script>")}, {"film.vtt", "auto", []byte(sample)},
		{"film.ass", "auto", []byte(sample)}, {"film.srt", "unknown", []byte(sample)},
		{"film.srt", "utf-8", append([]byte(sample), 0xff)}, {"film.srt", "auto", append([]byte(sample), 0)},
		{"bad\n.srt", "auto", []byte(sample)},
	} {
		if _, _, _, e := Normalize(c.name, c.charset, c.body); e == nil {
			t.Fatal("accepted invalid", c.name, c.charset)
		}
	}
	for _, c := range []struct{ name, text string }{{"f.vtt", "WEBVTT\n\n00:01.000 --> 00:02.000\ntext\n"}, {"f.ass", "[Script Info]\n[Events]\nDialogue: text\n"}, {"f.ssa", "[Script Info]\n[Events]\n"}} {
		if _, ext, _, e := Normalize(c.name, "auto", []byte(c.text)); e != nil || ContentType(ext) == "" {
			t.Fatal(c.name, e)
		}
	}
	expanded := append([]byte(sample), bytes.Repeat([]byte{0xd6, 0xd0}, (MaxBytes-len(sample))/2)...)
	if _, _, _, e := Normalize("film.srt", "gb18030", expanded); e == nil {
		t.Fatal("converted content >5MiB accepted")
	}
	for _, m := range []Metadata{{Revision: 0}, {Revision: 1, Language: "<script>"}, {Revision: 1, Title: "bad\ntext"}, {Revision: 1, Title: string(bytes.Repeat([]byte("a"), 201))}} {
		if m.Validate() == nil {
			t.Fatal(m)
		}
	}
	m := Metadata{Revision: 1, Language: " ZH-CN ", Title: " title "}
	if m.Validate() != nil || m.Language != "zh-cn" || m.Title != "title" {
		t.Fatal(m)
	}
}
