package scheduling

import "log/slog"

type Logger struct{ Log *slog.Logger }

func (l Logger) Info(message string, keysAndValues ...any) { l.Log.Debug(message, keysAndValues...) }
func (l Logger) Error(err error, message string, keysAndValues ...any) {
	l.Log.Error(message, append([]any{"error", err}, keysAndValues...)...)
}
