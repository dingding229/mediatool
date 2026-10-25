package imageedit

import (
	"context"
	"errors"
	cleanup "media-tool/internal/platform/cleanup"
	database "media-tool/internal/platform/database"
	"media-tool/internal/platform/identity"
	"os"
	"path/filepath"
	"time"
)

func (r Repository) remove(name string) error {
	if !safeFile(name) {
		return ErrInvalid
	}
	root, e := r.openRoot()
	if e != nil {
		return e
	}
	defer cleanup.Close(root, "image cleanup root")
	e = root.Remove(name)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}

// Reconcile scans at most one bounded directory batch per call. Only old,
// UUID-named files without a DB registration can be unfinished publications.
func (r Repository) Reconcile(ctx context.Context) error {
	if e := r.Collect(ctx); e != nil {
		return e
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer cleanup.Rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(840035)`); e != nil {
		return e
	}
	root, e := r.openRoot()
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer cleanup.Close(root, "image reconcile root")
	dir, e := root.Open(".")
	if e != nil {
		return e
	}
	defer cleanup.Close(dir, "image reconcile directory")
	entries, e := dir.ReadDir(100)
	if e != nil && len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if !safeFile(entry.Name()) {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		var registered bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_image_files WHERE filename=$1)`, entry.Name()).Scan(&registered); e != nil {
			return e
		}
		if registered {
			var referenced bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_image_overrides WHERE images @> jsonb_build_object('Primary',$1::text) OR images @> jsonb_build_object('Backdrop',$1::text) OR images @> jsonb_build_object('Logo',$1::text))`, entry.Name()[:36]).Scan(&referenced); e != nil {
				return e
			}
			if !referenced {
				if e = root.Remove(entry.Name()); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `DELETE FROM media_image_files WHERE filename=$1`, entry.Name()); e != nil {
					return e
				}
			}
		} else {
			if e = root.Remove(entry.Name()); e != nil {
				return e
			}
		}
	}
	return database.Commit(ctx, tx)
}
func validUUID(s string) bool { return identity.UUID(s) }

// Validate both the program directory and its parent before scoped read/delete.
// A configured/replaced symlink must never redirect collection into media/STRM.
func (r Repository) openRoot() (*os.Root, error) {
	path := r.root()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "images" || filepath.Base(filepath.Dir(path)) != "native-media" {
		return nil, ErrInvalid
	}
	for _, p := range []string{filepath.Dir(path), path} {
		info, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInvalid
		}
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		return nil, e
	}
	held, e := root.Stat(".")
	current, statErr := os.Lstat(path)
	parent, parentErr := os.Lstat(filepath.Dir(path))
	if e != nil || statErr != nil || parentErr != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(held, current) {
		cleanup.Close(root, "unsafe image root")
		return nil, ErrInvalid
	}
	return root, nil
}
