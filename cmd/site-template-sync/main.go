// Register all compiled Go templates, with no runtime source selection.
package main

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/database"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, e := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil || db == nil {
		os.Exit(1)
	}
	defer db.Close()
	if e = (sites.Repository{DB: db}).RegisterGoTemplates(ctx); e != nil {
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"registered": len(sites.GoTemplates()), "compiledGo": true})
}
