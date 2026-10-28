package embyapi

import (
	"encoding/json"
	"media-tool/internal/modules/mediausers/contracts"
	"testing"
)

func TestI10UserPolicyHasNoParentalRestriction(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		raw, err := json.Marshal(authUserDTO(contracts.User{ID: "owned", Enabled: enabled, CanPlay: true, MaxStreams: 3}, "server"))
		if err != nil {
			t.Fatal(err)
		}
		var value struct{ Policy map[string]any }
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"MaxParentalRating", "BlockUnratedItems"} {
			if _, ok := value.Policy[key]; ok {
				t.Fatal("restriction emitted", key)
			}
		}
		if value.Policy["SimultaneousStreamLimit"] != float64(3) || value.Policy["IsDisabled"] == enabled {
			t.Fatal("other user policy changed", value.Policy)
		}
	}
}
