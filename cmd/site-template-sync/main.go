// Load explicitly selected compiled Go templates, or refresh existing selections.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"time"
)

func main() {
	ids := flag.String("ids", "", "comma-separated template IDs to load; omit to refresh selected templates")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, e := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil || db == nil {
		os.Exit(1)
	}
	defer db.Close()
	repo := sites.Repository{DB: db}
	if *ids != "" {
		e = repo.LoadGoTemplates(ctx, strings.Split(*ids, ","))
	} else {
		e = repo.RegisterGoTemplates(ctx)
	}
	if e != nil {
		os.Exit(1)
	}
	loaded, e := repo.LoadedGoTemplates(ctx)
	if e != nil {
		os.Exit(1)
	}
	if checkedErr := json.NewEncoder(os.Stdout).Encode(map[string]any{"loaded": len(loaded), "compiledGo": true}); checkedErr != nil {
		os.Exit(1)
	}
}
