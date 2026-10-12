package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestLevelsAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		level string
		count int
	}{{"debug", 4}, {"info", 3}, {"warn", 2}, {"error", 1}} {
		t.Run(tc.level, func(t *testing.T) {
			var out bytes.Buffer
			l, err := New(&out, tc.level, "known-secret-fixture")
			if err != nil {
				t.Fatal(err)
			}
			l.Debug("debug")
			l.Info("info known-secret-fixture https://cdn.example/file?key=private", "accessToken", "opaque-secret", "error", errors.New("hidden-password"))
			l.Warn("warn")
			l.Error("error")
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != tc.count {
				t.Fatal(out.String())
			}
			for _, line := range lines {
				var obj map[string]any
				if json.Unmarshal([]byte(line), &obj) != nil {
					t.Fatal(line)
				}
				severity, ok := obj["level"].(string)
				if !ok || !strings.Contains("DEBUG INFO WARN ERROR", severity) || severity == "" {
					t.Fatal("missing structured severity", obj["level"])
				}
			}
			for _, private := range []string{"known-secret-fixture", "opaque-secret", "hidden-password", "key=private"} {
				if strings.Contains(out.String(), private) {
					t.Fatal("secret leaked")
				}
			}
		})
	}
	if _, err := New(&bytes.Buffer{}, "verbose"); err == nil {
		t.Fatal("invalid level accepted")
	}
}
func TestGroupsAndKnownSecrets(t *testing.T) {
	var out bytes.Buffer
	l, _ := New(&out, "debug")
	l.WithGroup("provider").With("password", "fixture").Info("event", slog.Group("nested", slog.String("refreshToken", "private")))
	if strings.Contains(out.String(), "fixture") || strings.Contains(out.String(), "private") {
		t.Fatal("group leaked")
	}
}

func TestOwnerRequestedRawPrivateLogsPreserveURLsAndValues(t *testing.T) {
	var out bytes.Buffer
	logger, e := NewRawControlled(&out, "debug", nil)
	if e != nil {
		t.Fatal(e)
	}
	logger.Info("302", "module", "playback", "url", "https://owned.example/strm?apiKey=fixture-only", "location", "https://cdn.example/file?sign=fixture-sign", "apiKey", "fixture-key")
	if !strings.Contains(out.String(), "fixture-sign") || !strings.Contains(out.String(), "fixture-key") || !strings.Contains(out.String(), "fixture-only") {
		t.Fatal(out.String())
	}
}
