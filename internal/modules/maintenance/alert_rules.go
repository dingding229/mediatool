package maintenance

import (
	"media-tool/internal/platform/outbox"
	"time"
)

const (
	OfflineQueueAlertThreshold  = 1
	PluginJournalAlertThreshold = outbox.PluginJournalWarning
)

type alertRule struct {
	code, key, message, unit string
	threshold                float64
	below                    bool
}

// Both evaluation and API presentation use these exact thresholds and direction.
func alertRules(p Policy) []alertRule {
	return []alertRule{
		{"plugin_journal", "pluginJournalUsed", "插件事件journal容量高，请核对订阅游标/unknown任务与保留窗口", "entries", PluginJournalAlertThreshold, false},
		{"memory", "memoryPercent", "宿主内存使用率过高", "percent", p.MemoryPercent, false},
		{"disk", "diskFreeGB", "STRM所在文件系统剩余空间不足", "GiB", p.DiskFreeGB, true},
		{"load", "loadPerCPU", "宿主每CPU负载过高", "load_per_cpu", p.LoadPerCPU, false},
		{"workers", "offlineQueues", "业务Worker离线", "queues", OfflineQueueAlertThreshold, false},
		{"failures", "failedJobs10m", "十分钟内失败任务过多", "jobs_per_10m", float64(p.FailedJobs), false},
	}
}

type Alert struct {
	Code           string     `json:"code"`
	State          string     `json:"state"`
	Hits           int        `json:"hits"`
	Value          *float64   `json:"value"`
	FirstSeenAt    *time.Time `json:"first_seen_at"`
	LastSeenAt     *time.Time `json:"last_seen_at"`
	LastSentAt     *time.Time `json:"last_sent_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	Message        string     `json:"message"`
	Threshold      *float64   `json:"threshold,omitempty"`
	Unit           string     `json:"unit,omitempty"`
	Comparison     string     `json:"comparison,omitempty"`
}
