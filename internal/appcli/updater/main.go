// Package updater is the labelled temporary-container entrypoint, never a web command.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/onlineupdate"
	"os"
	"time"
)

func Main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "updater requires admitted task ID")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	if e := onlineupdate.RunUpdater(ctx, os.Args[1]); e != nil {
		fmt.Fprintln(os.Stderr, "online updater stopped; inspect private task state")
		os.Exit(1)
	}
}

func Policy() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proof, e := onlineupdate.PolicyProof(ctx)
	if e != nil {
		fmt.Fprintln(os.Stderr, "Docker policy check unavailable")
		os.Exit(1)
	}
	if e = json.NewEncoder(os.Stdout).Encode(proof); e != nil {
		os.Exit(1)
	}
}
