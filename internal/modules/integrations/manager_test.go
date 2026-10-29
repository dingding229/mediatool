package integrations

import (
	"media-tool/internal/platform/config"
	"testing"
	"time"
)

func TestConnectionRevisionsDoNotRequireRestart(t *testing.T) {
	m := NewManager(Repository{}, config.Config{})
	defer m.Close()
	for i := 0; i < 80; i++ {
		c := Credentials{Record: Record{ID: "same-provider", UpdatedAt: time.Unix(int64(i), 0), Settings: map[string]any{"endpoint": "http://127.0.0.1:1"}}, Values: map[string]string{"token": "fixture"}}
		if _, err := m.cloudClient(c); err != nil {
			t.Fatal(i, err)
		}
	}
	if len(m.cloud) != 1 {
		t.Fatal("retained every revision")
	}
}

func TestExternalEmbyKindAndSettingsRemainRejected(t *testing.T) {
	for _, in := range []Input{
		{Kind: "emby", Name: "retired", Settings: map[string]any{"endpoint": "https://retired.invalid"}},
		{Kind: "open115", Name: "owned", Settings: map[string]any{"gatewayEnabled": true}},
		{Kind: "clouddrive2", Name: "owned", Settings: map[string]any{"directPlayOnly": true}},
	} {
		if Validate(in) == nil {
			t.Fatal("external Emby setting accepted", in.Kind)
		}
	}
}
