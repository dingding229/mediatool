package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"media-tool/internal/platform/backupverify"
	"media-tool/internal/platform/database"
	"net/url"
	"os"
	"regexp"
	"time"
)

func main() {
	source := flag.Bool("source-snapshot", false, "export a held read-only source snapshot for pg_dump")
	name := flag.String("database", "", "exact owned restore database")
	test := flag.Bool("test-only", false, "allow only media_tool_test")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if target := os.Getenv("BACKUP_VERIFY_DB"); target != "" {
		if target != *name || !regexp.MustCompile(`^media_tool_restore_[a-f0-9]{12}$`).MatchString(target) {
			os.Exit(1)
		}
		u, e := url.Parse(dsn)
		if e != nil {
			os.Exit(1)
		}
		u.Path = "/" + target
		dsn = u.String()
	}
	db, e := database.Open(ctx, dsn)
	if e != nil || db == nil {
		fmt.Fprintln(os.Stderr, "Restore verification connection failed; credentials omitted")
		os.Exit(1)
	}
	defer db.Close()
	if *source {
		out, tx, e := backupverify.ExportSnapshot(ctx, db)
		if e != nil {
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(out)
		bufio.NewReader(os.Stdin).ReadString('\n')
		tx.Rollback(ctx)
		return
	}
	r, e := backupverify.Verify(ctx, db, dsn, *name, os.Getenv("APP_SECRET_KEY"), *test)
	if e != nil {
		fmt.Fprintln(os.Stderr, "Restore verification failed; clone guard, migration or ciphertext validation did not pass. No credentials exported.")
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(r)
}
