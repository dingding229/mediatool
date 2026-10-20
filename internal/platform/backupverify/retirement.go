package backupverify

import "errors"

// This is a comparison policy, not a ciphertext/restore whitelist. Empty table
// removal is authorized only at migration 68. ExportSnapshot keeps every column.
func retirementProjection(before map[string]Table) (map[string]Table, error) {
	retired := map[string]bool{
		"plugin_legacy_wasm_plugins": true, "plugin_legacy_wasm_deliveries": true, "plugin_legacy_wasm_runs": true,
		"plugin_legacy_wasm_repositories": true, "plugin_legacy_wasm_jobs": true, "plugin_legacy_wasm_service_ticks": true,
		"site_accounts": true, "notification_channels": true, "bootstrap_imports": true, "media_user_legacy_grants": true,
	}
	columns := map[string]string{"download_tasks": "site_account_id", "notification_deliveries": "channel_id", "site_definitions": "local_overrides"}
	out := make(map[string]Table, len(before))
	for name, table := range before {
		if retired[name] {
			if table.Rows != 0 {
				return nil, errors.New("legacy retirement requires empty tables; restore/inspect backup")
			}
			continue
		}
		selected := []string{}
		for _, c := range table.Columns {
			if c != columns[name] {
				selected = append(selected, c)
			}
		}
		table.Columns = selected
		out[name] = table
	}
	return out, nil
}
