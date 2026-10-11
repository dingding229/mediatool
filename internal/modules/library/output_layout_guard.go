package library

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrOutputLayoutRequired = errors.New("已有输出与当前入库布局不同；请先恢复旧输出，再在云盘索引明确预览/确认迁移，不自动追加新旧布局")

// This guard belongs to the same asset-locked staging transaction. It checks
// the whole existing set before any alias, NFO or index can be added.
func CheckOutputLayout(ctx context.Context, tx pgx.Tx, assetID string, projections []OutputProjection) error {
	if len(projections) < 1 || len(projections) > 100 {
		return ErrOutputLayoutRequired
	}
	desired := map[string]bool{}
	for _, p := range projections {
		if !validRelativeArtifact(p.Path) || desired[p.Path] {
			return ErrOutputLayoutRequired
		}
		desired[p.Path] = true
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, assetID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path FOR UPDATE`, assetID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var current string
		if err = rows.Scan(&current); err != nil {
			return err
		}
		if !desired[current] {
			return ErrOutputLayoutRequired
		}
	}
	if count > 0 && count != len(desired) {
		return ErrOutputLayoutRequired
	}
	return rows.Err()
}
