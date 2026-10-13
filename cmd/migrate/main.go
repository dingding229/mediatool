package main

import (
	"context"
	"database/sql"
	"flag"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"log/slog"
	"media-tool/migrations"
	"os"
	"time"
)

func main() {
	cleanupPlugins := flag.Bool("cleanup-test-plugins", false, "clear only isolated test plugin fixtures before a new suite")
	testOnly := flag.Bool("test-only", false, "refuse databases other than media_tool_test")
	action := flag.String("action", "up", "up, status, or down (destructive)")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("database configuration invalid")
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		slog.Error("database unavailable")
		os.Exit(1)
	}
	if *cleanupPlugins && !*testOnly {
		os.Exit(1)
	}
	if *testOnly {
		var name string
		if err = db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "media_tool_test" {
			slog.Error("test-only migration requires isolated media_tool_test")
			os.Exit(1)
		}
	}
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if *cleanupPlugins && *action == "up" {
		var exists bool
		if err = db.QueryRowContext(ctx, `SELECT to_regclass('public.extension_plugins') IS NOT NULL`).Scan(&exists); err != nil {
			os.Exit(1)
		}
		if exists {
			if _, err = db.ExecContext(ctx, `TRUNCATE extension_plugins CASCADE`); err != nil {
				os.Exit(1)
			}
		}
	}
	switch *action {
	case "up":
		err = goose.UpContext(ctx, db, ".")
		if err == nil && *cleanupPlugins {
			// Browser suites use a fresh master key; never let their encrypted
			// proxy fixture leak into the next isolated Go/rekey test suite.
			_, err = db.ExecContext(ctx, `UPDATE network_proxy_settings SET tmdb_enabled=false,sites_enabled=false,url_ciphertext=''::bytea,revision=1 WHERE id='main'`)
		}
	case "status":
		err = goose.StatusContext(ctx, db, ".")
	case "down":
		err = goose.DownContext(ctx, db, ".")
	default:
		slog.Error("unknown migration action")
		os.Exit(1)
	}

	if err != nil {
		slog.Error("migration failed (details intentionally omitted; verify schema and connectivity)")
		os.Exit(1)
	}
}
