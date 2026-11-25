package maintenance

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"media-tool/internal/platform/backupverify"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/database"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var databaseOpen = database.Open

func cleanupFailedCandidate(committed *bool, result *error, cleanup func() error) {
	if !*committed {
		if err := cleanup(); err != nil {
			*result = errors.Join(*result, fmt.Errorf("restore candidate cleanup failed: %w", err))
		}
	}
}

func (e Engine) restoreClone(ctx context.Context, name, dir string, m Manifest) (string, func() error, error) {
	return e.restoreCloneChecked(ctx, name, dir, m, true)
}
func (e Engine) restoreCloneChecked(ctx context.Context, name, dir string, m Manifest, checkCipher bool) (dsn string, cleanup func() error, err error) {
	noop := func() error { return nil }
	if !strings.HasPrefix(name, "media_tool_restore_") || len(name) != len("media_tool_restore_")+12 {
		return "", noop, ErrUnsafe
	}
	for _, c := range name[len("media_tool_restore_"):] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", noop, ErrUnsafe
		}
	}
	u, err := url.Parse(e.DSN)
	if err != nil || strings.TrimPrefix(u.Path, "/") == name {
		return "", noop, ErrUnsafe
	}
	if _, err = e.DB.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`); err != nil {
		return "", noop, err
	}
	u.Path = "/" + name
	dsn = u.String()
	ok := false
	cleaned := false
	ownedCleanup := func() error {
		if cleaned {
			return nil
		}
		cleaned = true
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, dropErr := e.DB.Exec(clean, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH(FORCE)`)
		return dropErr
	}
	defer cleanupFailedCandidate(&ok, &err, ownedCleanup)
	dump, err := os.OpenFile(filepath.Join(dir, "database.dump"), os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return "", noop, err
	}
	err = e.pg(ctx, "pg_restore", dsn, []string{"--schema=public", "--exit-on-error", "--no-owner", "--no-acl", "--dbname=" + name}, dump, io.Discard)
	deferutil.Close(dump, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err != nil {
		return "", noop, err
	}
	if checkCipher {
		if _, err = e.verifyRestored(ctx, dsn, name, m, false); err != nil {
			return "", noop, fmt.Errorf("备份密文或迁移校验失败，已拒绝恢复：%w", err)
		}
	} else {
		// Automatic compensation must return even an already damaged source as
		// it was. Only our just-created same-schema snapshot may use this path.
		if m.Schema != buildinfo.Schema {
			return "", noop, ErrUnsafe
		}
		candidate, openErr := database.Open(ctx, dsn)
		if openErr != nil {
			return "", noop, openErr
		}
		actual, checkErr := Inventory(ctx, candidate)
		candidate.Close()
		if checkErr != nil || !sameTables(m.Tables, actual) {
			return "", noop, errors.New("恢复前快照库存校验失败")
		}
	}
	ok = true
	return dsn, ownedCleanup, nil
}

func restoreUID(fallback int) int {
	if value, err := strconv.Atoi(os.Getenv("OPS_RESTORE_UID")); err == nil && value > 0 {
		return value
	}
	return fallback
}
func restoreGID(fallback int) int {
	if value, err := strconv.Atoi(os.Getenv("OPS_RESTORE_GID")); err == nil && value >= 0 {
		return value
	}
	return fallback
}

// verifyRestored is shared by clone/drill and the lock-scoped same-database
// restore fixture. Test mode is accepted only by backupverify's exact DB guard.
func (e Engine) verifyRestored(ctx context.Context, dsn, name string, m Manifest, testOnly bool) (backupverify.Report, error) {
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return backupverify.Report{}, err
	}
	defer db.Close()
	actual, err := Inventory(ctx, db)
	if err != nil {
		return backupverify.Report{}, err
	}
	if !sameTables(m.Tables, actual) {
		return backupverify.Report{}, errors.New("restored database differs from backup table digest")
	}
	return backupverify.Verify(ctx, db, dsn, name, e.Key, testOnly)
}
