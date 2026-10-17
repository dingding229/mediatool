//go:build !linux && !darwin

package ffx

import "os"

func helperBlock(string) { os.Exit(2) }
