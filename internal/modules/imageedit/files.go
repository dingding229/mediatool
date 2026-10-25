package imageedit

import (
	"context"
	"errors"
	cleanup "media-tool/internal/platform/cleanup"
	database "media-tool/internal/platform/database"
	"media-tool/internal/platform/identity"
	"os"
	"time"
)

func (r Repository) remove(name string) error {
	if !safeFile(name) {
		return ErrInvalid
	}
	root, e := os.OpenRoot(r.root())
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
	root, e := os.OpenRoot(r.root())
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
