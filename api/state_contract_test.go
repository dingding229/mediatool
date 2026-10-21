package api_test

import "strings"

// String-backed persisted states are not Go named constants. Keep the owning
// state-machine vocabularies explicit; upstream/provider qbState remains open.
func annotateStateContracts(schemas object) {
	states := map[string][]string{
		"modules/search.Run":                         {"pending", "running", "succeeded", "partial", "failed"},
		"modules/search.SiteResult":                  {"pending", "running", "succeeded", "partial", "failed"},
		"modules/subscriptions.RunRecord":            {"pending", "running", "succeeded", "failed", "cancelled"},
		"modules/subscriptions.HistoryRun":           {"pending", "running", "succeeded", "failed", "cancelled"},
		"modules/manualdownloads.Task":               {"pending", "submitting", "downloading", "completed", "uncertain", "blocked", "absent_confirmed"},
		"modules/integrations.OfficialAuthorization": {"pending", "waiting", "scanned", "exchanging", "authorized", "expired", "cancelled", "failed"},
		"modules/integrations.CheckResult":           {"ready", "unavailable"},
		"modules/jobs.Job":                           {"pending", "running", "retry", "succeeded", "failed", "paused", "cancelled", "unknown", "checking"},
		"modules/jobs.Detail":                        {"pending", "running", "retry", "succeeded", "failed", "paused", "cancelled", "unknown", "checking"},
		"modules/library.Scan":                       {"pending", "running", "succeeded", "failed", "paused", "cancelled"},
		"modules/filemanager.Batch":                  {"preview", "running", "succeeded", "partial", "needs_reconcile", "expired"},
		"modules/filemanager.Result":                 {"preview", "running", "succeeded", "needs_reconcile", "failed", "expired"},
		"modules/plugins/v2.JournalCapacity":         {"ready", "warning", "blocked"},
		"modules/plugins/v2.StateSnapshot":           {"writing", "ready", "failed", "deleted"},
		"modules/nativeplayback.ActiveSession":       {"playing", "paused", "stopped"},
	}
	for _, value := range schemas {
		m, ok := value.(object)
		if !ok {
			continue
		}
		name, _ := m["x-go-type"].(string)
		name = strings.TrimPrefix(name, "media-tool/internal/")
		props, _ := m["properties"].(object)
		if props == nil {
			continue
		}
		if values := states[name]; len(values) > 0 {
			if p, ok := props["state"].(object); ok {
				p["enum"] = values
			}
		}
	}
}
