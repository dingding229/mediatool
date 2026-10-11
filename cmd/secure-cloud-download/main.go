package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "HTTPS preference update was not confirmed; existing provider permissions are required. Credentials and raw provider details omitted.")
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "disable only useHttpDownload on the configured cloud")
	flag.Parse()
	if !*apply {
		return fmt.Errorf("explicit apply required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
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
	c, err := manager.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return err
	}
	client, err := manager.CloudByID(ctx, c.Record.ID)
	if err != nil {
		return err
	}
	changed, err := client.RequireHTTPSDownloads(ctx, c.String("cloudRoot"))
	if err != nil {
		return err
	}
	if changed {
		if _, err = db.Exec(ctx, `UPDATE integrations SET revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$2`, c.Record.ID, c.Record.Revision); err != nil {
			return err
		}
		if _, err = db.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('authorized-operator','clouddrive.https_download',jsonb_build_object('integrationId',$1::text))`, c.Record.ID); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"httpDownloadPreferenceDisabled": true, "changed": changed, "otherProviderSettingsUnchanged": true, "passwordsChanged": false})
}
