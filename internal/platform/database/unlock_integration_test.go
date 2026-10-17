package database

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestFailedUnlockDiscardsPhysicalSession(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("media_tool_test only")
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pid := conn.Conn().PgConn().PID()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended('media-tool-test-unlock',0))`); err != nil {
		t.Fatal(err)
	}
	Unlock(ctx, conn, `SELECT false`)
	conn.Release()
	other, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	if other.Conn().PgConn().PID() == pid {
		t.Fatal("failed-unlock connection returned to pool")
	}
	var locked bool
	if err = other.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('media-tool-test-unlock',0))`).Scan(&locked); err != nil || !locked {
		t.Fatal("lock leaked", err)
	}
	Unlock(ctx, other, `SELECT pg_advisory_unlock(hashtextextended('media-tool-test-unlock',0))`)
}
