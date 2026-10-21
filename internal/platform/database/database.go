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
	// Retain the ten-connection capacity required by maintenance/writer gates,
	// but do not keep hundreds of large query plans per idle connection.
	if cfg.ConnConfig.StatementCacheCapacity > 64 {
		cfg.ConnConfig.StatementCacheCapacity = 64
	}
	if cfg.ConnConfig.DescriptionCacheCapacity > 64 {
		cfg.ConnConfig.DescriptionCacheCapacity = 64
	}
	if cfg.MaxConnIdleTime > 2*time.Minute {
		cfg.MaxConnIdleTime = 2 * time.Minute
	}
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	// Pool creation is lazy. Readiness reports failures without preventing startup.
	return pgxpool.NewWithConfig(ctx, cfg)
}
