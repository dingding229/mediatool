//go:build !linux && !darwin

package stillstore

import (
	"context"
	"os"
)

// Fail closed: never fall back to a check-then-rename that can overwrite.
func openRoot(string) (*os.File, error)                    { return nil, ErrUnavailable }
func openShard(*os.File, string, bool) (*os.File, error)   { return nil, ErrUnavailable }
func openLeaf(*os.File, string) (*os.File, error)          { return nil, ErrUnavailable }
func temporary(*os.File) (*os.File, string, error)         { return nil, "", ErrUnavailable }
func lockRoot(context.Context, *os.File) (*os.File, error) { return nil, ErrUnavailable }
func renameExclusive(*os.File, string, string) error       { return ErrUnavailable }
func discard(*os.File, string, *error)                     {}
