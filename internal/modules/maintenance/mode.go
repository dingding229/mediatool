package maintenance

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Paused(ctx context.Context, db *pgxpool.Pool) (bool, error) {
	if db == nil {
		return false, nil
	}
	var paused bool
	err := db.QueryRow(ctx, `SELECT paused FROM ops_mode WHERE singleton`).Scan(&paused)
	return paused, err
}
