package requestbudget

import "time"

const (
	DefaultResolve       = 30 * time.Second
	MinConfiguredResolve = 5 * time.Second
	MaxConfiguredResolve = time.Minute
)

// EffectiveResolve also supports short injected budgets in deterministic tests.
// User configuration is separately validated against Min/MaxConfiguredResolve.
func EffectiveResolve(value time.Duration) time.Duration {
	if value <= 0 {
		return DefaultResolve
	}
	return min(value, MaxConfiguredResolve)
}

// The fixed resolver performs no unbounded stream writes. Keep its response
// deadline finite but allow lookup, DNS validation, a bounded CDN sample, reply.
func FixedSTRMWrite(value time.Duration) time.Duration {
	return EffectiveResolve(value) + 17*time.Second
}
func InspectWrite(value time.Duration) time.Duration { return EffectiveResolve(value) + 22*time.Second }
