package v2

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/platform/scheduling"
	"time"
)

// Validate only explicitly marked, compiled form fields, never infer cron from
// arbitrary configuration keys. Runtime publication/dispatch handles exhaustion
// separately; an explicit install/configure must still have a future execution.
func validateCronConfig(pluginID string, raw json.RawMessage, now time.Time) error {
	var inventory struct {
		Forms map[string]contracts.Form `json:"forms"`
	}
	if json.Unmarshal(libraryInventory, &inventory) != nil {
		return ErrUnavailable
	}
	form := inventory.Forms[pluginID]
	fields := []string{}
	var walk func([]contracts.Component)
	walk = func(nodes []contracts.Component) {
		for _, n := range nodes {
			if n.Component == "VCronInput" {
				fields = append(fields, n.Model)
			}
			walk(n.Children)
		}
	}
	walk(form.Content)
	if len(fields) == 0 {
		return nil
	}
	values := map[string]json.RawMessage{}
	input := map[string]json.RawMessage{}
	if json.Unmarshal(form.Defaults, &values) != nil || json.Unmarshal(raw, &input) != nil || input == nil {
		return ErrInvalid
	}
	for k, v := range input {
		values[k] = v
	}
	for _, field := range fields {
		var expression string
		if json.Unmarshal(values[field], &expression) != nil {
			return ErrInvalid
		}
		if _, err := scheduling.Next(expression, now, time.Minute); err != nil {
			return ErrInvalid
		}
	}
	return nil
}
