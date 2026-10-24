// Package subtitles owns administrator-uploaded text subtitles and per-asset
// metadata overlays. It never uploads/deletes a provider file or changes media.
package subtitles

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/mediaext"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 5 << 20
const MaxIndex = 2147483647

var (
	ErrInvalid     = errors.New("字幕参数、类型或内容无效")
	ErrNotFound    = errors.New("字幕或物理版本不存在")
	ErrConflict    = errors.New("字幕已被修改，请刷新后重试")
	ErrUnavailable = errors.New("字幕存储暂不可用")
)

func Root(c config.Config) string {
	base := c.NativeStillsRoot
	if base == "" {
		base = "/data/native-media/stills"
	}
	return path.Join(path.Dir(base), "subtitles")
}

type Metadata struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
	Hidden   bool   `json:"hidden"`
	Revision int64  `json:"revision"`
}
type Entry struct {
	Metadata
	ID          string `json:"id"`
	AssetID     string `json:"assetId"`
	Source      string `json:"source"`
	SourceKey   string `json:"-"`
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	Extension   string `json:"extension"`
	StoragePath string `json:"-"`
	Deleting    bool   `json:"deleting"`
	Overridden  bool   `json:"-"`
	Digest      []byte `json:"-"`
	Size        int64  `json:"size"`
	Encoding    string `json:"encoding,omitempty"`
}
type Version struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Page struct {
	AssetID  string    `json:"assetId"`
	Versions []Version `json:"versions"`
	Items    []Entry   `json:"items"`
}

var languagePattern = regexp.MustCompile(`^[a-zA-Z]{2,3}(?:-[a-zA-Z]{2,4})?$`)

func (m *Metadata) Validate() error {
	m.Language = strings.ToLower(strings.TrimSpace(m.Language))
	m.Title = strings.TrimSpace(m.Title)
	if m.Revision < 1 || m.Language != "" && !languagePattern.MatchString(m.Language) || !utf8.ValidString(m.Title) || utf8.RuneCountInString(m.Title) > 200 || strings.ContainsAny(m.Title, "\x00\r\n") {
		return ErrInvalid
	}
	if _, e := json.Marshal(dto.MediaStream{Language: m.Language, DisplayTitle: m.Title}); e != nil {
		return ErrInvalid
	}
	return nil
}
func Infer(name string) Metadata {
	lower := "." + strings.ToLower(name)
	m := Metadata{Title: bounded(path.Base(name), 200), Default: strings.Contains(lower, ".default."), Forced: strings.Contains(lower, ".forced.") || strings.Contains(lower, ".foreign."), Revision: 1}
	for _, lang := range []string{"zh-cn", "zh-tw", "chs", "cht", "zho", "chi", "zh", "eng", "en", "jpn", "ja", "kor", "ko", "fra", "fre", "deu", "ger", "spa", "rus"} {
		if strings.Contains(lower, "."+lang+".") {
			m.Language = lang
			break
		}
	}
	return m
}
func Matches(video, sub string) bool {
	if path.Dir(video) != path.Dir(sub) || !mediaext.IsSubtitle(sub) {
		return false
	}
	stem := strings.ToLower(strings.TrimSuffix(path.Base(video), path.Ext(video)))
	name := strings.ToLower(strings.TrimSuffix(path.Base(sub), path.Ext(sub)))
	return name == stem || strings.HasPrefix(name, stem+".")
}

func bounded(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\x00", ""))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
