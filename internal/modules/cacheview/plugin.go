package cacheview

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"strings"
)

// Invalidate removes only explicit safe cache keys. Search proof snapshots and
// media/download records are retained. There is no Clear/FLUSH surface.
func (s Store) Invalidate(ctx context.Context, kind, id string) error {
	p, e := prefix(kind)
	if e != nil {
		return e
	}
	key := p + id
	if kind == "search" {
		if !uuidID.MatchString(id) {
			return ErrInvalid
		}
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			return e
		}
		defer deferutil.Rollback(ctx, tx)
		if e = tx.QueryRow(ctx, `UPDATE search_runs SET cache_hidden=true WHERE id=$1 RETURNING coalesce(cache_key,'')`, id).Scan(&key); e != nil {
			return e
		}
		if key != "" && (!strings.HasPrefix(key, p) || !hexID.MatchString(strings.TrimPrefix(key, p))) {
			return ErrInvalid
		}
		if e = dbutil.Commit(ctx, tx); e != nil {
			return e
		}
	} else if !hexID.MatchString(id) {
		return ErrInvalid
	}
	if e = dbutil.CheckEffect(ctx); e != nil {
		return e
	}
	if key == "" {
		return nil
	}
	if s.Redis == nil {
		return errors.New("cache unavailable")
	}
	return s.Redis.Del(ctx, key).Err()
}
