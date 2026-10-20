// media-tool-dev contains build/test tooling, never the application runtime.
package main

import (
	"fmt"
	"media-tool/internal/tooling"
	"os"
)

func main() {
	if err := tooling.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
