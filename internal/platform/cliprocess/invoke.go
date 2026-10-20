// Package cliprocess adapts process-only commands, never request handlers. It
// preserves each leaf's flags/exit behavior without duplicate standalone binaries.
package cliprocess

import (
	"flag"
	"os"
)

func Invoke(name string, args []string, main func()) {
	oldArgs, oldFlags := os.Args, flag.CommandLine
	defer func() { os.Args, flag.CommandLine = oldArgs, oldFlags }()
	os.Args = append([]string{oldArgs[0]}, args...)
	flag.CommandLine = flag.NewFlagSet(name, flag.ExitOnError)
	main()
}
