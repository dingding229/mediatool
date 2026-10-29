package api_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestI27RetiredPhasesRemovedAndNativeContractsRemain(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for _, key := range []string{"FlowStatus", "platform_flow_Status"} {
		phase := schemas[key].(map[string]any)["properties"].(map[string]any)["phase"].(map[string]any)
		for _, value := range phase["enum"].([]any) {
			if value == "emby_refresh" || value == "emby_verify" {
				t.Fatal("retired phase", key, value)
			}
		}
	}
	// DTOs, auth schemes and native client routes are not external integrations.
	if _, ok := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)["EmbyToken"]; !ok {
		t.Fatal("native auth contract removed")
	}
	native := 0
	for key := range schemas {
		if len(key) >= 8 && key[:8] == "embyapi_" {
			native++
		}
	}
	// Schema naming can include its package segment; reject losing the native DTO surface.
	if native == 0 {
		for key := range schemas {
			if len(key) >= 16 && key[:16] == "modules_embyapi_" {
				native++
			}
		}
	}
	if native == 0 {
		t.Fatal("native protocol DTO contracts removed")
	}
}
