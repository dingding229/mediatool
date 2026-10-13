// Explicit native-template sync; never reads PT secrets or executes downloads.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/database"
	"os"
	"time"
)

func main() {
	commit := flag.String("source-commit", "", "exact mediatool-site 40-character commit")
	license := flag.Bool("license-accepted", false, "operator confirms native repository template license")
	flag.Parse()
	if !*license || *commit == "" || sites.ValidateImport(nil, *commit) != nil {
		fmt.Fprintln(os.Stderr, "An exact native-repository commit and license confirmation are required.")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, e := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil || db == nil {
		os.Exit(1)
	}
	defer db.Close()
	definitions, e := (sites.Repository{DB: db}).ImportSnapshot(ctx, nil, *commit)
	if e != nil {
		fmt.Fprintln(os.Stderr, "Native template sync failed; read saved definitions before retrying; credentials omitted.")
		os.Exit(1)
	}
	ids := []string{}
	for _, d := range definitions {
		ids = append(ids, d.ID)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"repository": sites.NativeRepository, "sourceCommit": *commit, "templateIds": ids, "accountsChanged": false})
}
