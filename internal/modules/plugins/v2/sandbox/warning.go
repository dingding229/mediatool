package sandbox

import "log/slog"

func WarnUnavailable(logger *slog.Logger, instance string) {
	logger.Warn("Plugin Landlock unavailable; trusted-code mode", "module", "plugins", "instance", instance, "reason", "landlock_unavailable")
}
