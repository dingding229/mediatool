package api_test

import "testing"

func TestI44PluginHistoryReadOnlyOperationName(t *testing.T) {
	d := readDocument(t)
	schemas := d["components"].(object)["schemas"].(object)
	found := false
	for name, raw := range schemas {
		if name == "modules_plugins_v2_Run" {
			props := raw.(object)["properties"].(object)
			if props["operationName"] == nil {
				t.Fatal("missing operationName")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing plugin run contract")
	}
}
