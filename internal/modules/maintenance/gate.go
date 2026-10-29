package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/platform/workgate"
)

// BeginWork shares the same lightweight admission fence as background workers.
func BeginWork(ctx context.Context, db *pgxpool.Pool) (func(), error) {
	return workgate.BeginWork(ctx, db)
}
