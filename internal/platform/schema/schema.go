// Package schema applies embedded upward migrations inside the main process.
package schema

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"media-tool/migrations"
)

func Up(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("invalid migration configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	if err = db.PingContext(ctx); err != nil {
		return errors.New("database unavailable for startup migration")
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("migration lock unavailable")
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker), goose.WithLogger(goose.NopLogger()))
	if err != nil {
		return errors.New("migration provider unavailable")
	}
	if _, err = p.Up(ctx); err != nil {
		return errors.New("startup migration failed; inspect schema and backup without exposing credentials")
	}
	return nil
}
