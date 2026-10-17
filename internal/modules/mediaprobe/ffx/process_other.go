//go:build !linux && !darwin

package ffx

import "os/exec"

// Refuse execution rather than silently losing process-tree cancellation.
func configureGroup(*exec.Cmd) error  { return ErrUnavailable }
func stopGroup(*exec.Cmd, bool) error { return ErrUnavailable }
