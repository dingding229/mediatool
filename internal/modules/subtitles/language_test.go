package subtitles

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"strings"
	"testing"
)

func TestFilenameLanguageMarkers(t *testing.T) {
	for _, c := range []struct {
		markers         []string
		language, label string
	}{{[]string{"chs", "sc", "zh-hans", "gb", "zh-cn"}, "chi", "简体中文"}, {[]string{"cht", "tc", "zh-hant", "big5", "zh-tw"}, "chi", "繁体中文"}, {[]string{"en", "eng"}, "eng", "英语"}, {[]string{"ja", "jp", "jpn"}, "jpn", "日语"}, {[]string{"ko", "kor"}, "kor", "韩语"}, {[]string{"chs&eng", "cht&eng", "简英", "中英", "繁英"}, "chi", "中英双语"}} {
		for _, marker := range c.markers {
			t.Run(marker, func(t *testing.T) {
				d := FilenameLanguage("Where.the.Mask.Ends.2026.S01E06." + marker + ".srt")
				if !d.Known || d.Language != c.language || d.Label != c.label {
					t.Fatal(d)
				}
			})
		}
	}
	for _, name := range []string{"French.srt", "EnglishMovie.srt", "Scary.chsTitle.srt", "normal.srt"} {
		if d := FilenameLanguage(name); d.Known {
			t.Fatal(name, d)
		}
	}
}
func cue(text string) []byte { return []byte("1\n00:00:01,000 --> 00:00:03,000\n" + text + "\n") }
func TestLanguageContentAndEncoding(t *testing.T) {
	for _, c := range []struct {
		name, text, language, label string
		codec                       encoding.Encoding
	}{{"UTF8简体", "这是简体中文字幕，欢迎观看今天的电影。", "chi", "简体中文", nil}, {"UTF16简体", "这是简体中文字幕，欢迎观看今天的电影。", "chi", "简体中文", unicodeenc.UTF16(unicodeenc.LittleEndian, unicodeenc.UseBOM)}, {"GBK简体", "这是简体中文字幕，欢迎观看今天的电影。", "chi", "简体中文", simplifiedchinese.GB18030}, {"Big5繁体", "這是繁體中文字幕，歡迎觀看臺灣電影。", "chi", "繁体中文", traditionalchinese.Big5}, {"UTF8日本語", "こんにちは、これは日本語の字幕です。", "jpn", "日语", nil}, {"ShiftJIS日本語", "こんにちは、これは日本語の字幕です。", "jpn", "日语", japanese.ShiftJIS}, {"英语", "John: Welcome to the film. Thank you for watching!", "eng", "英语", nil}, {"韩语", "안녕하세요 이것은 한국어 자막입니다", "kor", "韩语", nil}, {"未知", "1234567 !? ☃", "und", "未知", nil}} {
		t.Run(c.name, func(t *testing.T) {
			raw := cue(c.text)
			if c.codec != nil {
				var e error
				raw, e = c.codec.NewEncoder().Bytes(raw)
				if e != nil {
					t.Fatal(e)
				}
			}
			d, e := DetectLanguage("release.srt", "auto", raw)
			if e != nil || d.Language != c.language || d.Label != c.label || d.Known != (c.language != "und") {
				t.Fatal(d, e)
			}
		})
	}
}
func TestFilenameEvidenceWinsWithoutChangingContent(t *testing.T) {
	raw := cue("This is an English subtitle.")
	d, e := DetectLanguage("file.chs.srt", "auto", raw)
	if e != nil || d.Language != "chi" || !strings.Contains(d.Evidence, "文件名") {
		t.Fatal(d, e)
	}
	d, e = DetectLanguage("file.ass", "auto", []byte("[Script Info]\nTitle: English header\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,這是繁體中文電影字幕"))
	if e != nil || d.Label != "繁体中文" {
		t.Fatal(d, e)
	}
}

func TestLanguageDetectionAtBoundedLegacySampleBoundary(t *testing.T) {
	text := "1\n00:00:01,000 --> 00:00:02,000\na" + strings.Repeat("這是繁體中文電影字幕歡迎觀看測試", 8000)
	raw, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= 65536 {
		t.Fatal("sample must truncate a multibyte stream")
	}
	result, err := DetectLanguage("sample.srt", "auto", raw)
	if err != nil || result.Language != "chi" || result.Label != "繁体中文" || result.Encoding != "big5" {
		t.Fatal(result, err)
	}
}

func TestLanguageMarkerDoesNotForceFileEncoding(t *testing.T) {
	for _, c := range []struct {
		name, text, charset string
		codec               encoding.Encoding
	}{
		{"film.cht.srt", "這是繁體中文電影字幕歡迎觀看測試", "gb18030", simplifiedchinese.GB18030},
		{"film.chs.srt", "這是繁體中文電影字幕歡迎觀看測試", "big5", traditionalchinese.Big5},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw, e := c.codec.NewEncoder().Bytes([]byte("1\n00:00:01,000 --> 00:00:02,000\n" + c.text))
			if e != nil {
				t.Fatal(e)
			}
			result, e := DetectLanguage(c.name, "auto", raw)
			if e != nil || result.Encoding != c.charset {
				t.Fatal(result, e)
			}
		})
	}
}

func TestUnspecifiedChineseScriptHasNeutralDisplayLabel(t *testing.T) {
	if got := LanguageLabel("chi", nil); got != "中文" {
		t.Fatal(got)
	}
	if got := DisplayTitle(Metadata{Language: "chi"}, "srt"); strings.Contains(got, "待核对") || !strings.Contains(got, "中文") {
		t.Fatal(got)
	}
}
