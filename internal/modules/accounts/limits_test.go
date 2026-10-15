package accounts

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
)

type limitCleaner struct {
	sql string
	err error
}

func (c *limitCleaner) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	c.sql = sql
	return pgconn.NewCommandTag("DELETE 0"), c.err
}
func TestAttemptCleanupStaysBoundedAndPropagatesFailure(t *testing.T) {
	want := errors.New("unavailable")
	db := &limitCleaner{err: want}
	if err := pruneExpiredAttempts(context.Background(), db); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if !strings.Contains(db.sql, "LIMIT 256") || !strings.Contains(db.sql, "ORDER BY window_start") || !strings.Contains(db.sql, "window_start<now()-interval '1 day'") {
		t.Fatal("unbounded or stale-window-unaware cleanup", db.sql)
	}
	db.err = nil
	if err := pruneExpiredAttempts(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
