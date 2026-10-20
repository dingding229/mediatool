package main

import (
	"fmt"
	"media-tool/internal/operator"
	"os"
)

func main() {
	if e := operator.Run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
