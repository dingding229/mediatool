// Owner-only CLI. It is not an HTTP command endpoint and never accepts shell code.
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"media-tool/internal/modules/externalapi"
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
	confirm := flags.Bool("confirm", false, "confirm temporary status.read credential verification and cleanup")
	id := flags.String("id", "", "maintenance/backup UUID")
	revision := flags.String("revision", "", "verified source revision")
	digest := flags.String("digest", "", "image digest")
	schema := flags.Int64("schema", 0, "verified schema")
	success := flags.Bool("success", false, "completed safely")
	confirmText := flags.String("confirm-text", "", "exact preview confirmation phrase")
	message := flags.String("message", "", "owner execution result")
	targets := flags.String("targets", "", "explicit movie:id,series:id classification repair targets (max 20)")
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
	if action == "external-jwt-check" {
		if !*confirm {
			return maintenance.ErrUnsafe
		}
		check, stop := context.WithTimeout(ctx, 45*time.Second)
		defer stop()
		out, e := externalapi.VerifyLocalLifecycle(check, db, c.SecretKey, c.Address)
		if encodeErr := json.NewEncoder(os.Stdout).Encode(out); encodeErr != nil {
			return encodeErr
		}
		return e
	}
	if action == "refresh-classification" {
		return refreshClassification(ctx, db, c, *targets, os.Stdout)
	}
	e := maintenance.Engine{DB: db, DSN: c.DatabaseURL, Root: c.MaintenanceDir, OutputRoot: c.STRMOutputDir, Key: c.SecretKey, HostProc: c.HostProcDir, Revision: buildinfo.Revision}
	var out any
	switch action {
	case "restore-preview":
		var r maintenance.WebRestore
		r, err = e.PreviewWebRestore(ctx, *id)
		out = maintenance.RestorePreview(r)
	case "restore-execute":
		var r maintenance.WebRestore
		r, err = e.QueueWebRestore(ctx, *id, *digest, *confirmText)
		if err == nil {
			r, err = e.WaitWebRestore(ctx, r.ID)
		}
		out = r
	case "restore-status":
		out, err = e.WebRestore(ctx, *id)
	case "restore-reconcile":
		out, err = e.ReconcileWebRestore(ctx, *id)
	case "restore-request":
		out, err = e.RestoreApprovedRequest(ctx, *id)
	case "register-release":
		err = e.RegisterRelease(ctx, maintenance.Release{Revision: *revision, Image: "dingding229/mediatool:sha-" + *revision, Schema: *schema, Digest: *digest, Verified: true})
		out = map[string]bool{"registered": err == nil}
	case "check":
		out, err = e.CheckRequest(ctx, *id)
	case "claim":
		var req maintenance.Request
		req, err = e.Request(ctx, *id)
		if err == nil {
			err = requireImageControl(req.Kind)
		}
		if err == nil {
			out, err = e.Claim(ctx, *id)
		}
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
		err = errors.New("旧低层数据恢复入口已停用；请使用restore-preview/restore-execute或已批准维护单的restore-request，共用app内恢复器")
	case "finish":
		var req maintenance.Request
		req, err = e.Request(ctx, *id)
		if err == nil {
			err = requireImageControl(req.Kind)
		}
		if err == nil {
			err = e.Finish(ctx, *id, *message, *success)
		}
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

// An old host media-toolctl must fail before stopping app or swapping a DB.
// The supported high-level data restore entry points all use the shared engine.
func requireImageControl(kind string) error {
	if kind == "restore" {
		return errors.New("数据恢复必须使用app内共享执行器；请更新media-toolctl或运行restore-preview/restore-execute")
	}
	return nil
}
