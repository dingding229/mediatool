package messaging

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var noticeUUID = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
var noticeInternalLine = regexp.MustCompile(`(?i)^\s*(?:asset_?id|batch_?id|operation_?id|event_?id|notification_?id)\s*[:：]`)
var noticeMessagePrefix = regexp.MustCompile(`(?i)^\s*message\s*[:：]\s*`)
var noticeMetric = regexp.MustCompile(`指标\s+([a-zA-Z_][a-zA-Z0-9_]*)[，,]\s*阈值\s*([0-9]+(?:\.[0-9]+)?)`)
var neutralNoticeNames = strings.NewReplacer(
	"Emby 媒体库刷新已请求", "媒体库更新已请求",
	"媒体已在Emby库中可见（待客户端播放验收）", "媒体库已更新（待播放器验证）",
	"托管输出逐路径可见（待完整播放验收）", "媒体库已更新（待播放器验证）",
)

// This is a read-only UI projection. Stored messages and the delivery worker's body are never changed.
func presentNotice(kind string, payload []byte, title, body string) (string, string) {
	title = cleanNoticeText(title)
	if neutral, ok := map[string]string{"library.refresh_requested": "媒体库更新已请求", "library.visible": "媒体库已更新（待播放器验证）", "library.emby.visible": "媒体库已更新（待播放器验证）"}[kind]; ok {
		title = neutral
	}
	if kind == "ops.alert.active" || kind == "ops.alert.recovered" {
		if message := presentNoticeAlert(kind, payload, body); message != "" {
			return title, message
		}
		if kind == "ops.alert.recovered" {
			return title, "系统指标告警已解除。"
		}
		return title, "系统指标触发告警，请在运维页核对。"
	}
	body = cleanNoticeText(body)
	if body == title {
		body = ""
	}
	return title, body
}
func cleanNoticeText(text string) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if noticeInternalLine.MatchString(line) {
			continue
		}
		line = noticeMessagePrefix.ReplaceAllString(line, "")
		line = neutralNoticeNames.Replace(line)
		line = strings.TrimSpace(noticeUUID.ReplaceAllString(line, ""))
		line = strings.Trim(line, " \t：:；;")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
func noticeNumber(value any) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case string:
		var e error
		n, e = strconv.ParseFloat(v, 64)
		if e != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func presentNoticeAlert(kind string, payload []byte, body string) string {
	var data map[string]any
	_ = json.Unmarshal(payload, &data)
	metric, _ := data["metric"].(string)
	threshold, ok := noticeNumber(data["threshold"])
	if match := noticeMetric.FindStringSubmatch(body); len(match) == 3 {
		if metric == "" {
			metric = match[1]
		}
		if !ok {
			threshold, ok = noticeNumber(match[2])
		}
	}
	type display struct {
		name, unit string
		below      bool
	}
	fields := map[string]display{
		"memoryPercent": {"内存使用率", "%", false}, "diskFreeGB": {"剩余空间", " GiB", true},
		"loadPerCPU": {"每 CPU 负载", "", false}, "offlineQueues": {"离线执行器", " 个", false},
		"failedJobs10m": {"十分钟失败任务", " 个", false}, "pluginJournalUsed": {"插件事件记录", " 条", false},
	}
	f, known := fields[metric]
	if !known || !ok {
		return ""
	}
	format := func(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) + f.unit }
	if kind == "ops.alert.recovered" {
		return f.name + "告警已解除（阈值 " + format(threshold) + "）"
	}
	if value, exists := noticeNumber(data["value"]); exists {
		relation := "超过"
		if f.below {
			relation = "低于"
		}
		if value == threshold {
			relation = "达到"
		}
		return f.name + format(value) + "，" + relation + "阈值" + format(threshold)
	}
	// Old events did not retain the measured value: never borrow today's metric to invent a historic value.
	return f.name + "触发告警（阈值 " + format(threshold) + "）"
}
