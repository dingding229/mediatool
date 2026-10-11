package integrations

import "testing"

func TestEmbyDirectOnlyIsAnExplicitBooleanSetting(t *testing.T) {
	for _, value := range []any{true, false} {
		if err := Validate(Input{Kind: "emby", Name: "owned", Settings: map[string]any{"directPlayOnly": value}}); err != nil {
			t.Fatal(err)
		}
	}
	if Validate(Input{Kind: "emby", Name: "owned", Settings: map[string]any{"directPlayOnly": "true"}}) == nil {
		t.Fatal("string bypassed boolean policy")
	}
	if Validate(Input{Kind: "tmdb", Name: "owned", Settings: map[string]any{"directPlayOnly": true}}) == nil {
		t.Fatal("policy allowed on other integration")
	}
}
