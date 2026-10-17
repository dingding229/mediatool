package main

import (
	"context"
	"flag"
	"fmt"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Administrator password reset was not confirmed. Check the selected database, account, private password file and offline acknowledgement; sensitive details omitted.")
		os.Exit(1)
	}
}
func run() error {
	username := flag.String("username", "", "existing administrator username")
	file := flag.String("password-file", "", "0600 local file containing the new password; no password argument")
	offline := flag.Bool("offline-confirmed", false, "confirm application and all workers are stopped")
	flag.Parse()
	if !*offline {
		return fmt.Errorf("offline acknowledgement required")
	}
	name, err := accounts.NormalizeUsername(*username)
	if err != nil {
		return err
	}
	stat, err := os.Lstat(*file)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > 514 {
		return fmt.Errorf("private regular password file required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	hash, err := auth.HashPassword(password)
	password = ""
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil || db == nil {
		return fmt.Errorf("database required")
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var id string
	if err = tx.QueryRow(ctx, `UPDATE administrator_accounts SET password_hash=$2,credential_version=credential_version+1,must_change_password=false,temporary_password_expires_at=NULL,updated_at=now()WHERE username=$1 RETURNING id::text`, name, hash).Scan(&id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_sessions SET revoked_at=now()WHERE account_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM administrator_recovery_codes WHERE account_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('offline-operator','account.password_reset',jsonb_build_object('accountId',$1::text))`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Println("Administrator password reset committed; all prior sessions are revoked. Public registration remains closed. No password value printed.")
	return nil
}
