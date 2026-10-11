package database

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, nil
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	// Pool creation is lazy. Readiness reports failures without preventing startup.
	return pgxpool.NewWithConfig(ctx, cfg)
}
