// Package logging provides the application's built-in structured log system.
// JSON goes to stdout; Docker's bounded json-file driver owns disk rotation.
package logging

import (
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

var urlPattern = regexp.MustCompile(`(?i)https?://[^\s"<>]+`)
var credentialPattern = regexp.MustCompile(`(?i)(bearer\s+\S+|(?:password|api[_-]?key|access[_-]?token|refresh[_-]?token|recovery[_-]?code|cookie|authorization|secret)[=:]\s*[^\s,;]+)`)

func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
}
func New(out io.Writer, level string, knownSecrets ...string) (*slog.Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	clean := func(value string) string {
		for _, secret := range knownSecrets {
			if len(secret) >= 6 {
				value = strings.ReplaceAll(value, secret, "[redacted]")
			}
		}
		value = urlPattern.ReplaceAllString(value, "[url omitted]")
		value = credentialPattern.ReplaceAllString(value, "[credential omitted]")
		if len(value) > 4096 {
			value = value[:4096] + " [truncated]"
		}
		return value
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: parsed, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		key := strings.ToLower(a.Key)
		for _, sensitive := range []string{"password", "secret", "token", "cookie", "authorization", "apikey", "api_key", "recoverycode", "recovery_code", "dsn", "database_url", "redis_url", "signedurl"} {
			if strings.Contains(key, sensitive) {
				return slog.String(a.Key, "[redacted]")
			}
		}
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindAny { // Never serialize arbitrary errors or payload objects.
			a.Value = slog.StringValue("[details omitted]")
		} else if a.Value.Kind() == slog.KindString {
			a.Value = slog.StringValue(clean(a.Value.String()))
		}
		return a
	}})), nil
}
