// Package trace correlates administrator-visible business activity without
// recording login bodies or changing provider execution behavior.
package trace

import (
	"context"
	"log/slog"
)

type key struct{}

func With(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, logger)
}
func Log(ctx context.Context, level slog.Level, module, message string, fields ...any) {
	logger, _ := ctx.Value(key{}).(*slog.Logger)
	if logger == nil {
		return
	}
	logger.With("module", module).Log(ctx, level, message, fields...)
}
func Info(ctx context.Context, module, message string, fields ...any) {
	Log(ctx, slog.LevelInfo, module, message, fields...)
}
func Warn(ctx context.Context, module, message string, fields ...any) {
	Log(ctx, slog.LevelWarn, module, message, fields...)
}
