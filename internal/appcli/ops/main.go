// Owner-only CLI. It is not an HTTP command endpoint and never accepts shell code.
package ops

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"os"
	"time"
)

func Main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return maintenance.ErrUnsafe
	}
	action := os.Args[1]
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	id := flags.String("id", "", "maintenance/backup UUID")
	revision := flags.String("revision", "", "verified source revision")
	digest := flags.String("digest", "", "image digest")
	schema := flags.Int64("schema", 0, "verified schema")
	success := flags.Bool("success", false, "completed safely")
	message := flags.String("message", "", "owner execution result")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if action == "info" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema": buildinfo.Schema, "revision": buildinfo.Revision, "version": buildinfo.Version})
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, c.DatabaseURL)
	if err != nil || db == nil {
		return maintenance.ErrUnsafe
	}
	defer db.Close()
	e := maintenance.Engine{DB: db, DSN: c.DatabaseURL, Root: c.MaintenanceDir, OutputRoot: c.STRMOutputDir, Key: c.SecretKey, HostProc: c.HostProcDir, Revision: buildinfo.Revision}
	var out any
	switch action {
	case "register-release":
		err = e.RegisterRelease(ctx, maintenance.Release{Revision: *revision, Image: "dingding229/mediatool:sha-" + *revision, Schema: *schema, Digest: *digest, Verified: true})
		out = map[string]bool{"registered": err == nil}
	case "check":
		out, err = e.CheckRequest(ctx, *id)
	case "claim":
		out, err = e.Claim(ctx, *id)
	case "backup":
		var backup string
		backup, err = e.QueueBackup(ctx, "owner-preflight")
		if err == nil {
			err = wait(ctx, func() (string, error) {
				var state string
				err := db.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1`, backup).Scan(&state)
				return state, err
			}, "ready")
		}
		out = map[string]string{"backupId": backup}
	case "drill":
		var drill string
		drill, err = e.QueueDrill(ctx, *id)
		if err == nil {
			err = wait(ctx, func() (string, error) {
				var state string
				err := db.QueryRow(ctx, `SELECT state FROM ops_drills WHERE id=$1`, drill).Scan(&state)
				return state, err
			}, "verified")
		}
		out = map[string]string{"drillId": drill}
	case "verify-execution":
		out, err = e.VerifyExecution(ctx, *id)
	case "verify":
		out, _, err = e.VerifyBundle(ctx, *id)
	case "prepare":
		var candidate string
		candidate, err = e.PrepareCandidate(ctx, *id)
		out = map[string]string{"candidateDatabase": candidate}
	case "finish":
		err = e.Finish(ctx, *id, *message, *success)
		out = map[string]bool{"recorded": err == nil}
	default:
		return maintenance.ErrUnsafe
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func wait(ctx context.Context, state func() (string, error), success string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		v, err := state()
		if err != nil {
			return err
		}
		if v == success {
			return nil
		}
		if v != "pending" && v != "running" {
			return fmt.Errorf("维护任务停止: %s", v)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
