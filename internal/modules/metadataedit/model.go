// Package metadataedit owns administrator text overlays. Provider and file-index
// writers never write this table; removing a key reveals the current base value.
package metadataedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalid = errors.New("元数据输入无效")
var ErrConflict = errors.New("信息或条目绑定已变化，请重新读取后比较保存")
var ErrNotFound = errors.New("媒体条目不存在")
var ErrUnavailable = errors.New("元数据服务暂不可用")

type Values map[string]json.RawMessage

var Fields = []string{"title", "originalTitle", "sortName", "year", "premiereDate", "overview", "tagline", "genres", "tags", "communityRating", "officialRating"}

type Target struct {
	MediaID string `json:"mediaId"`
	Kind    string `json:"kind"`
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
}

func (t Target) Key() string {
	return fmt.Sprintf("%s:%s:%d:%d", t.MediaID, t.Kind, t.Season, t.Episode)
}

type Record struct {
	Target         Target   `json:"target"`
	TargetKey      string   `json:"targetKey"`
	Revision       int64    `json:"revision"`
	Base           Values   `json:"base"`
	Effective      Values   `json:"effective"`
	Overrides      Values   `json:"overrides"`
	Matched        bool     `json:"matched"`
	Unmatched      []Target `json:"unmatched"`
	Choices        []Target `json:"choices"`
	UnmatchedPage  int      `json:"unmatchedPage"`
	UnmatchedTotal int      `json:"unmatchedTotal"`
}
type Patch struct {
	TargetKey string   `json:"targetKey"`
	Revision  int64    `json:"revision"`
	Set       Values   `json:"set"`
	Restore   []string `json:"restore"`
}

func invalid(s string) error { return fmt.Errorf("%w：%s", ErrInvalid, s) }
func known(k string) bool {
	for _, f := range Fields {
		if f == k {
			return true
		}
	}
	return false
}
func (p *Patch) Validate() error {
	if p.Revision < 0 || p.TargetKey == "" || len(p.Set)+len(p.Restore) == 0 {
		return invalid("请提交有效的修订号和修改字段")
	}
	seen := map[string]bool{}
	for _, k := range p.Restore {
		if !known(k) || seen[k] {
			return invalid("恢复字段无效或重复")
		}
		seen[k] = true
	}
	for k, raw := range p.Set {
		if !known(k) || seen[k] {
			return invalid("修改字段无效或与恢复字段重复")
		}
		var value any
		switch k {
		case "genres", "tags":
			var a []string
			if string(raw) == "null" || json.Unmarshal(raw, &a) != nil || len(a) > 50 {
				return invalid("类型/标签最多50项，每项不超过100字符")
			}
			out := []string{}
			unique := map[string]bool{}
			for _, s := range a {
				s = strings.TrimSpace(s)
				if !utf8.ValidString(s) || strings.ContainsRune(s, 0) || utf8.RuneCountInString(s) > 100 {
					return invalid("类型/标签每项不超过100字符")
				}
				if s != "" && !unique[s] {
					out = append(out, s)
					unique[s] = true
				}
			}
			value = out
		case "year", "communityRating":
			if string(raw) == "null" {
				value = nil
				break
			}
			var n float64
			if json.Unmarshal(raw, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return invalid("年份或评分必须为数字")
			}
			if k == "year" {
				if n < 1800 || n > 2100 || n != math.Trunc(n) {
					return invalid("年份必须为1800～2100之间的整数")
				}
				value = int(n)
			} else {
				if n < 0 || n > 10 {
					return invalid("评分必须为0～10")
				}
				value = math.Round(n*10) / 10
			}
		default:
			if k == "premiereDate" && string(raw) == "null" {
				value = nil
				break
			}
			var s string
			if string(raw) == "null" || json.Unmarshal(raw, &s) != nil || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
				return invalid("文字字段格式不正确")
			}
			s = strings.TrimSpace(s)
			limit := 500
			if k == "overview" {
				limit = 10000
			}
			if k == "tagline" {
				limit = 1000
			}
			if utf8.RuneCountInString(s) > limit {
				return invalid(fmt.Sprintf("%s不能超过%d字符", fieldLabel(k), limit))
			}
			if k == "title" && s == "" {
				return invalid("标题不能为空")
			}
			if k == "premiereDate" && s != "" {
				if _, e := time.Parse("2006-01-02", s); e != nil {
					return invalid("日期必须为合法的YYYY-MM-DD日期")
				}
			}
			if k == "premiereDate" && s == "" {
				value = nil
			} else {
				value = s
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return invalid("字段无法保存")
		}
		p.Set[k] = encoded
	}
	return nil
}
func fieldLabel(k string) string {
	return map[string]string{"title": "标题", "originalTitle": "原始标题", "sortName": "排序标题", "overview": "简介", "tagline": "标语", "officialRating": "分级", "premiereDate": "首播日期"}[k]
}
func Merge(base, overrides Values) Values {
	out := Values{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
