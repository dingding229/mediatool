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
	switch *action {
	case "up":
		err = goose.UpContext(ctx, db, ".")
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
