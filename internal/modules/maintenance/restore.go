package maintenance

import (
	"archive/tar"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"media-tool/internal/platform/backupverify"
	"media-tool/internal/platform/database"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var databaseOpen = database.Open

func (e Engine) restoreClone(ctx context.Context, name, dir string, m Manifest) (string, error) {
	if !strings.HasPrefix(name, "media_tool_restore_") || len(name) != len("media_tool_restore_")+12 {
		return "", ErrUnsafe
	}
	for _, c := range name[len("media_tool_restore_"):] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", ErrUnsafe
		}
	}
	u, err := url.Parse(e.DSN)
	if err != nil || strings.TrimPrefix(u.Path, "/") == name {
		return "", ErrUnsafe
	}
	if _, err = e.DB.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`); err != nil {
		return "", err
	}
	u.Path = "/" + name
	dsn := u.String()
	ok := false
	defer func() {
		if !ok {
			clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e.DB.Exec(clean, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH(FORCE)`)
		}
	}()
	dump, err := os.OpenFile(filepath.Join(dir, "database.dump"), os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return "", err
	}
	err = e.pg(ctx, "pg_restore", dsn, []string{"--exit-on-error", "--no-owner", "--no-acl", "--dbname=" + name}, dump, io.Discard)
	dump.Close()
	if err != nil {
		return "", err
	}
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return "", err
	}
	defer db.Close()
	actual, err := Inventory(ctx, db)
	if err != nil {
		return "", err
	}
	if !sameTables(m.Tables, actual) {
		return "", errors.New("恢复克隆与原备份摘要不一致")
	}
	if _, err = backupverify.Verify(ctx, db, dsn, name, e.Key, false); err != nil {
		return "", err
	}
	ok = true
	return dsn, nil
}
func (e Engine) restoreOutputs(ctx context.Context, archive string, proofs []FileProof) error {
	file, err := os.OpenFile(archive, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return err
	}
	defer file.Close()
	expected := map[string]FileProof{}
	for _, proof := range proofs {
		if !safeRelative(proof.Path) || !Digest.MatchString(proof.Digest) || proof.Bytes < 0 || proof.Bytes > 64<<20 {
			return ErrUnsafe
		}
		if _, ok := expected[proof.Path]; ok {
			return ErrUnsafe
		}
		expected[proof.Path] = proof
	}
	fs, err := os.OpenRoot(e.OutputRoot)
	if err != nil {
		return err
	}
	defer fs.Close()
	restore := map[string]bool{}
	seen := map[string]bool{}
	tr := tar.NewReader(file)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		p, ok := expected[h.Name]
		if !ok || seen[h.Name] || h.Typeflag != tar.TypeReg || h.Size != p.Bytes {
			return ErrUnsafe
		}
		seen[h.Name] = true
		raw, err := io.ReadAll(io.LimitReader(tr, p.Bytes+1))
		if err != nil || int64(len(raw)) != p.Bytes || hash(raw) != p.Digest {
			return ErrUnsafe
		}
		for parent := path.Dir(h.Name); parent != "."; parent = path.Dir(parent) {
			info, err := fs.Lstat(parent)
			if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
				return ErrUnsafe
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		info, err := fs.Lstat(h.Name)
		if err == nil {
			if !info.Mode().IsRegular() {
				return ErrUnsafe
			}
			src, err := fs.OpenFile(h.Name, os.O_RDONLY|noFollowFlag(), 0)
			if err != nil {
				return err
			}
			current, err := io.ReadAll(io.LimitReader(src, (64<<20)+1))
			src.Close()
			if err != nil || hash(current) != p.Digest {
				return errors.New("已有输出与备份不同，禁止覆盖: " + h.Name)
			}
		} else if os.IsNotExist(err) {
			restore[h.Name] = true
		} else {
			return err
		}
	}
	if len(seen) != len(expected) {
		return ErrUnsafe
	}
	if _, err = file.Seek(0, 0); err != nil {
		return err
	}
	tr = tar.NewReader(file)
	for {
		header, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		name := header.Name
		if !restore[name] {
			continue
		}
		proof := expected[name]
		raw, readErr := io.ReadAll(io.LimitReader(tr, proof.Bytes+1))
		if readErr != nil || int64(len(raw)) != proof.Bytes || hash(raw) != proof.Digest {
			return ErrUnsafe
		}

		missingDirs := []string{}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, statErr := fs.Lstat(parent); os.IsNotExist(statErr) {
				missingDirs = append(missingDirs, parent)
			}
		}
		if err = fs.MkdirAll(path.Dir(name), 0750); err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			if info, statErr := os.Stat(e.OutputRoot); statErr == nil {
				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					for _, parent := range missingDirs {
						if chownErr := os.Chown(filepath.Join(e.OutputRoot, parent), restoreUID(int(stat.Uid)), restoreGID(int(stat.Gid))); chownErr != nil {
							return chownErr
						}
					}
				}
			}
		}
		out, err := fs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollowFlag(), 0600)
		if err != nil {
			return err
		}
		_, err = out.Write(raw)
		syncErr := out.Sync()
		out.Close()
		if os.Geteuid() == 0 {
			if info, statErr := os.Stat(e.OutputRoot); statErr == nil {
				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					_ = os.Chown(filepath.Join(e.OutputRoot, name), restoreUID(int(stat.Uid)), restoreGID(int(stat.Gid)))
				}
			}
		}
		if err != nil {
			return err
		}
		if syncErr != nil {
			return syncErr
		}
	}
	return nil
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
