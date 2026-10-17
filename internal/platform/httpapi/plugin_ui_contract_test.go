package httpapi

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"testing"
)

func TestPluginUISharedGoVueContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/plugin-v2-ui.json")
	if err != nil {
		t.Fatal(err)
	}
	var x struct {
		Form contracts.Form            `json:"form"`
		Page contracts.Page            `json:"page"`
		Meta []contracts.DashboardMeta `json:"meta"`
	}
	if contracts.Decode(raw, &x) != nil || x.Form.Content[0].Model != "greeting" || len(x.Meta) != 2 || x.Form.Content[2].When.Model != "scheduled" {
		t.Fatal("Go/Vue contract", string(raw))
	}
	if _, err = json.Marshal(x); err != nil {
		t.Fatal(err)
	}
}
