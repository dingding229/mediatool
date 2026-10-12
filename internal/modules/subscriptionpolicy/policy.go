// Package subscriptionpolicy shares the original subscription criteria with
// explicit download-source recovery; recovery cannot bypass free/H&R rules.
package subscriptionpolicy

import (
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releaseattrs"
	"media-tool/internal/modules/search"
	"regexp"
	"strings"
	"unicode"
)

type Rules struct {
	Resolution     string   `json:"resolution,omitempty"`
	Source         string   `json:"source,omitempty"`
	Codec          string   `json:"codec,omitempty"`
	AccountIDs     []string `json:"accountIds"`
	Keyword        string   `json:"keyword"`
	Include        []string `json:"include"`
	Exclude        []string `json:"exclude"`
	MinSize        int64    `json:"minSize"`
	MaxSize        int64    `json:"maxSize"`
	MinSeeders     int      `json:"minSeeders"`
	AllowUnknownHR bool     `json:"allowUnknownHR"`
	FreeOnly       bool     `json:"freeOnly"`
	Season         int      `json:"season"`
	Episodes       []int    `json:"episodes"`
	IncludeHistory bool     `json:"includeHistory"`
}

func TitleMatches(title string, item metadata.Item) bool {
	trimmed := strings.TrimSpace(title)
	prefix := regexp.MustCompile(`^\[[^\]]+\]\s*`)
	for {
		next := prefix.ReplaceAllString(trimmed, "")
		if next == trimmed {
			break
		}
		trimmed = next
	}
	delimiter := regexp.MustCompile(`(?i)[\s._\[\(]+(?:19|20)\d{2}|[\s._\[\(]+S\d{1,3}(?:E\d{1,4})?`)
	if pos := delimiter.FindStringIndex(trimmed); pos != nil {
		trimmed = trimmed[:pos[0]]
	}
	normalize := func(s string) string {
		return strings.ToLower(strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, s))
	}
	actual := normalize(trimmed)
	if actual == "" {
		return false
	}
	if actual != normalize(item.Title) && actual != normalize(item.OriginalTitle) {
		return false
	}
	if item.MediaType == "movie" {
		years := regexp.MustCompile(`(?:19|20)\d{2}`).FindAllString(title, -1)
		if item.Year == "" {
			return false
		}
		found := false
		for _, y := range years {
			if y == item.Year {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func Evaluate(resource search.Resource, item metadata.Item, rules Rules) (bool, []string, []RuleCheck) {
	reasons := []string{}
	checks := AttributeChecks(resource.Title, rules)
	for _, check := range checks {
		if check.State == "rejected" {
			reasons = append(reasons, check.Message)
		}
	}
	lower := strings.ToLower(resource.Title)
	if !TitleMatches(resource.Title, item) {
		reasons = append(reasons, "标题/年份无法精确对应已选择媒体，需要人工确认")
	}
	for _, word := range rules.Include {
		if !strings.Contains(lower, strings.ToLower(word)) {
			reasons = append(reasons, "缺少包含词："+word)
		}
	}
	for _, word := range rules.Exclude {
		if strings.Contains(lower, strings.ToLower(word)) {
			reasons = append(reasons, "包含排除词："+word)
		}
	}
	if rules.MinSize > 0 && resource.Size < rules.MinSize {
		reasons = append(reasons, "体积过小或未知")
	}
	if rules.MaxSize > 0 && resource.Size > rules.MaxSize {
		reasons = append(reasons, "体积超过上限")
	}
	if rules.MinSeeders > 0 && resource.Seeders < rules.MinSeeders {
		reasons = append(reasons, "做种人数不足或未知")
	}
	if rules.FreeOnly && (resource.DownloadFactor == nil || *resource.DownloadFactor != 0) {
		reasons = append(reasons, "不是已确认的免费资源")
	}
	if hr, ok := resource.Metadata["hr"].(bool); ok && hr {
		reasons = append(reasons, "资源包含H&R约束，自动订阅不下载")
	} else if !ok && !rules.AllowUnknownHR {
		reasons = append(reasons, "H&R信息未知，需要人工确认或明确允许未知信息")
	}
	return len(reasons) == 0, reasons, checks
}

type RuleCheck struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	State    string `json:"state"`
	Message  string `json:"message"`
}

func AttributeChecks(title string, rules Rules) []RuleCheck {
	attrs := releaseattrs.Parse(title)
	checks := []RuleCheck{}
	for _, field := range []struct{ key, label, want string }{{"resolution", "分辨率", rules.Resolution}, {"source", "来源", rules.Source}, {"codec", "编码", rules.Codec}} {
		actual := attrs[field.key]
		check := RuleCheck{Key: field.key, Label: field.label, Expected: field.want, Actual: actual, State: "not_configured", Message: "未限制"}
		if field.want != "" {
			check.State = "matched"
			check.Message = field.label + "匹配：" + actual
			if actual == "" {
				check.State = "rejected"
				check.Message = field.label + "未知，不满足明确筛选"
			} else if actual != field.want {
				check.State = "rejected"
				check.Message = field.label + "不匹配：需要" + field.want + "，识别为" + actual
			}
		}
		checks = append(checks, check)
	}
	return checks
}
func Filter(resource search.Resource, item metadata.Item, rules Rules) (bool, []string) {
	matched, reasons, _ := Evaluate(resource, item, rules)
	return matched, reasons
}
