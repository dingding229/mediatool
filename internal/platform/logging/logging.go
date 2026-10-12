// Package logging provides the application's built-in structured log system.
// JSON is shared with stdout and the bounded file store used by the admin console.
package logging

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
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
	return NewControlled(out, level, nil, knownSecrets...)
}
func NewControlled(out io.Writer, level string, control *slog.LevelVar, knownSecrets ...string) (*slog.Logger, error) {
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
	var threshold slog.Leveler = parsed
	if control != nil {
		control.Set(parsed)
		threshold = control
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: threshold, AddSource: true, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.SourceKey {
			if source, ok := a.Value.Any().(*slog.Source); ok {
				if source == nil || source.File == "" || source.Line <= 0 {
					return slog.Attr{}
				}
				file := filepath.ToSlash(source.File)
				if index := strings.Index(file, "/internal/"); index >= 0 {
					file = file[index+1:]
				} else if index := strings.Index(file, "/cmd/"); index >= 0 {
					file = file[index+1:]
				} else {
					file = filepath.Base(file)
				}
				return slog.Group(a.Key, slog.String("file", clean(file)), slog.Int("line", source.Line))
			}
		}
		// The built-in level is a slog.Level (KindAny) with text marshaling.
		// Preserve this known enum, not arbitrary payload/error objects.
		if len(groups) == 0 && a.Key == slog.LevelKey {
			if _, ok := a.Value.Any().(slog.Level); ok {
				return a
			}
		}
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
