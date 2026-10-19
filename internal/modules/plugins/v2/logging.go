package v2

import (
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
)

// Only caller-reviewed metadata belongs here. Never attach RPC args/results,
// configuration, operation keys, credentials or raw errors to lifecycle logs.
func (s Store) logInfo(ctx context.Context, instance, message string, fields ...any) {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{"module", "plugins", "instance", instance}
	logger.InfoContext(ctx, message, append(attrs, fields...)...)
}
func callLogResult(err error) string {
	var rpc *contracts.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "unknown"
	}
	if errors.As(err, &rpc) {
		if rpc.Code == contracts.ResultUnknown || rpc.Code == contracts.Unavailable {
			return "unknown"
		}
		if rpc.Code == contracts.PermissionDenied {
			return "denied"
		}
	}
	return "failed"
}
