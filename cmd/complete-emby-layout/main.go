// Complete only the known deployment's media layout, never reset accounts or
// replace provider credentials. --apply is required for all writes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/library"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Media layout completion failed; private provider details omitted. Inspect state before retry.")
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "apply known /strm mapping and dedicated libraries")
	flag.Parse()
	if !*apply {
		return fmt.Errorf("explicit apply required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.STRMOutputDir != "/data/strm" {
		return fmt.Errorf("unexpected deployment root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil || db == nil {
		return fmt.Errorf("database required")
	}
	defer db.Close()
	vault, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg)
	defer manager.Close()
	emby, err := manager.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	if emby.Values["apiKey"] == "" {
		return fmt.Errorf("existing Emby key required")
	}
	if prefix := emby.String("strmPathPrefix"); prefix != "" && prefix != "/strm" {
		return fmt.Errorf("existing non-default mapping must not be overwritten")
	}
	httpClient := http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, endpoint string, body any, out any) error {
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = strings.NewReader(string(raw))
		}
		r, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(emby.String("endpoint"), "/")+endpoint, reader)
		if err != nil {
			return err
		}
		r.Header.Set("X-Emby-Token", emby.Values["apiKey"])
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		response, err := httpClient.Do(r)
		if err != nil {
			return err
		}
		defer deferutil.Close(response.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		if response.StatusCode != 200 && response.StatusCode != 204 {
			return fmt.Errorf("provider refused")
		}
		if out != nil {
			return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
		}
		if _, checkedErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4096)); checkedErr != nil {
			return checkedErr
		}
		return nil
	}
	type folder struct {
		Name      string   `json:"Name"`
		Locations []string `json:"Locations"`
	}
	var existing []folder
	if err = request("GET", "/Library/VirtualFolders", nil, &existing); err != nil {
		return err
	}
	root, err := os.OpenRoot(cfg.STRMOutputDir)
	if err != nil {
		return err
	}
	defer deferutil.Close(root, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	created := []string{}
	for _, plan := range []struct{ name, kind, sub string }{{"Media Tool 电影", "movies", "Movies"}, {"Media Tool 剧集", "tvshows", "TV"}} {
		target := "/strm/" + plan.sub
		found := false
		nameConflict := false
		for _, f := range existing {
			if f.Name == plan.name {
				nameConflict = true
			}
			for _, p := range f.Locations {
				if p == target {
					found = true
				}
			}
		}
		if found {
			continue
		}
		if nameConflict {
			return fmt.Errorf("dedicated library name already has different paths")
		}
		if err = root.MkdirAll(plan.sub, 0750); err != nil {
			return err
		}
		if err = request("POST", "/Library/VirtualFolders", map[string]any{"Name": plan.name, "CollectionType": plan.kind, "RefreshLibrary": false, "Paths": []string{target}}, nil); err != nil {
			return err
		}
		created = append(created, target)
	}
	if emby.String("strmPathPrefix") != "/strm" || emby.Record.Settings["gatewayEnabled"] != true {
		settings := map[string]any{}
		for k, v := range emby.Record.Settings {
			settings[k] = v
		}
		settings["strmPathPrefix"] = "/strm"
		settings["gatewayEnabled"] = true
		if _, err = manager.Repository.Save(ctx, emby.Record.ID, integrations.Input{Kind: emby.Record.Kind, Name: emby.Record.Name, Enabled: emby.Record.Enabled, Settings: settings, Revision: emby.Record.Revision}); err != nil {
			return err
		}
	}
	var verified []folder
	if err = request("GET", "/Library/VirtualFolders", nil, &verified); err != nil {
		return err
	}
	locations := []string{}
	for _, f := range verified {
		locations = append(locations, f.Locations...)
	}
	scanID, scanErr := (library.Engine{DB: db, Connections: manager}).Start(ctx, "")
	report := map[string]any{"accountChanged": false, "credentialsReplaced": false, "strmPathPrefix": "/strm", "gatewayEnabled": true, "createdLibraryPaths": created, "libraryLocations": locations, "scanQueued": scanErr == nil}
	if scanErr == nil {
		report["scanId"] = scanID
	}
	if checkedErr := json.NewEncoder(os.Stdout).Encode(report); checkedErr != nil {
		return checkedErr
	}
	return nil
}
