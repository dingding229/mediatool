package sandbox

import (
	"errors"
	"log/slog"
)

const AllowWithoutLandlockEnv = "MEDIATOOL_PLUGIN_ALLOW_NO_LANDLOCK"

var ErrLandlockUnavailable = errors.New("sandbox_unavailable")

// Allow only a known unavailable ABI, never an error applying the ruleset.
func RequireLandlock(active bool, applyErr error, allowUnsupported bool) error {
	if applyErr != nil || !active && !allowUnsupported {
		return ErrLandlockUnavailable
	}
	return nil
}

func WarnUnavailable(logger *slog.Logger, instance string) {
	logger.Warn("Plugin Landlock unavailable; explicit degraded mode", "module", "plugins", "instance", instance, "reason", "landlock_unavailable")
}
