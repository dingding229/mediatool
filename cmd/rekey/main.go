package main

import (
	"context"
	"flag"
	"fmt"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Rekey failed; raw database/key details omitted. Transaction was not confirmed; keep both keys and inspect before retry.")
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "commit rotation; default checks then rolls back")
	offline := flag.Bool("offline-confirmed", false, "acknowledge application and every worker are stopped")
	flag.Parse()
	if !*offline {
		return fmt.Errorf("offline acknowledgement required")
	}
	file := os.Getenv("APP_SECRET_KEY_NEXT_FILE")
	stat, err := os.Stat(file)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("next key requires private file")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	old, err := secrets.New(os.Getenv("APP_SECRET_KEY"))
	if err != nil {
		return err
	}
	next, err := secrets.New(strings.TrimSpace(string(raw)))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) == os.Getenv("APP_SECRET_KEY") {
		return fmt.Errorf("keys must differ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil || db == nil {
		return fmt.Errorf("database required")
	}
	defer db.Close()
	counts, err := secrets.Rekey(ctx, db, old, next, !*apply)
	if err != nil {
		return err
	}
	fmt.Printf("Rekey checked; committed=%t; ciphertext tables=%d. No key values printed. Update local APP_SECRET_KEY before restarting services after commit.\n", *apply, len(counts))
	return nil
}
