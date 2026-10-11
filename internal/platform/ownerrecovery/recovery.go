// Package ownerrecovery handles only an offline owner's empty password marker.
// The temporary password is printed to the startup console, never application
// log files, management responses or file browsing. No network reset endpoint.
package ownerrecovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sys/unix"
	"io"
	"media-tool/internal/platform/auth"
	"os"
	"path/filepath"
	"syscall"
)

func Apply(ctx context.Context, db *pgxpool.Pool, marker string, console io.Writer) (bool, error) {
	if marker == "" || db == nil {
		return false, nil
	}
	dir, err := os.OpenRoot(filepath.Dir(marker))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("owner recovery directory unavailable")
	}
	defer dir.Close()
	base := filepath.Base(marker)
	pending := "." + base + ".pending"
	info, err := dir.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		info, err = dir.Lstat(pending)
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("owner recovery marker unavailable")
	}
	if !info.Mode().IsRegular() || info.Size() != 0 || info.Mode().Perm()&0022 != 0 {
		return false, errors.New("password marker must be an empty regular file, not a link or group/world-writable")
	}
	if existing, e := dir.Lstat(".password-reset.lock"); e == nil && !existing.Mode().IsRegular() {
		return false, errors.New("unsafe owner recovery lock")
	}
	lock, err := dir.OpenFile(".password-reset.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, errors.New("owner recovery lock unavailable")
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return false, errors.New("owner recovery already active")
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	if current, e := dir.Lstat(base); e == nil {
		if !os.SameFile(info, current) || current.Size() != 0 {
			return false, errors.New("password marker changed")
		}
		if _, e = dir.Lstat(pending); !errors.Is(e, os.ErrNotExist) {
			return false, errors.New("pending recovery marker exists; inspect offline")
		}
		if e = dir.Rename(base, pending); e != nil {
			return false, errors.New("password marker claim failed")
		}
	}
	current, err := dir.Lstat(pending)
	if err != nil || !os.SameFile(info, current) || current.Size() != 0 || !current.Mode().IsRegular() {
		return false, errors.New("password marker changed during claim")
	}
	st, ok := current.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 {
		return false, errors.New("marker identity unavailable")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", st.Dev, st.Ino, current.ModTime().UnixNano())))
	identity := hex.EncodeToString(sum[:])
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, errors.New("owner recovery database unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(718)`); err != nil {
		return false, err
	}
	var used bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM administrator_bootstrap_resets WHERE marker_digest=$1)`, identity).Scan(&used); err != nil {
		return false, err
	}
	if used {
		if err = dir.Remove(pending); err != nil {
			return false, err
		}
		return false, nil
	}
	var id, username string
	if err = tx.QueryRow(ctx, `SELECT id::text,username FROM administrator_accounts FOR UPDATE`).Scan(&id, &username); err != nil {
		return false, errors.New("registered administrator required; marker retained")
	}
	bytes := make([]byte, 15)
	if _, err = rand.Read(bytes); err != nil {
		return false, err
	}
	temporary := base64.RawURLEncoding.EncodeToString(bytes)
	hash, err := auth.HashPassword(temporary)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_accounts SET password_hash=$2,credential_version=credential_version+1,must_change_password=true,temporary_password_expires_at=now()+interval '30 minutes',updated_at=now() WHERE id=$1`, id, hash); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_sessions SET revoked_at=now() WHERE account_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO administrator_bootstrap_resets(marker_digest,account_id)VALUES($1,$2)`, identity, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('offline-owner','account.owner_recovery',jsonb_build_object('accountId',$1::text))`, id); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, errors.New("owner recovery outcome unknown; inspect before retry")
	}
	if _, err = fmt.Fprintf(console, "[OWNER PASSWORD RECOVERY] Account: %s | Temporary password: %s | Expires in 30 minutes. Login must set a new password. This appears once in the startup console, never the web log viewer.\n", username, temporary); err != nil {
		return true, errors.New("owner password applied but console unavailable; create a new marker offline")
	}
	if err = dir.Remove(pending); err != nil {
		return true, errors.New("recovery applied; marker removal failed, do not reuse")
	}
	return true, nil
}
