package adminreset

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
)

func TestOfflineResetImmediatelyInvalidatesSlidingSession(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "t1_reset_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := base.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
	}()
	for _, table := range []string{"administrator_accounts", "administrator_sessions", "administrator_recovery_codes", "audit_logs"} {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, table}.Sanitize()+` (LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	repo := accounts.Repository{DB: db}
	key := auth.Auth{Key: strings.Repeat("a", 64)}
	token, e := key.Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256([]byte(token))
	if _, e = repo.Register(ctx, "t1_reset_admin", "original-password", token); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `UPDATE administrator_sessions SET expires_at=now()+interval '1 hour' WHERE token_digest=$1`, digest[:]); e != nil {
		t.Fatal(e)
	}
	var before time.Time
	if e = db.QueryRow(ctx, `SELECT expires_at FROM administrator_sessions WHERE token_digest=$1`, digest[:]).Scan(&before); e != nil {
		t.Fatal(e)
	}
	input, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	params := input.Query()
	params.Set("search_path", schema)
	input.RawQuery = params.Encode()
	t.Setenv("DATABASE_URL", input.String())
	file := filepath.Join(t.TempDir(), "new-password")
	if e = os.WriteFile(file, []byte("replacement-password"), 0600); e != nil {
		t.Fatal(e)
	}
	previousFlags, previousArgs := flag.CommandLine, os.Args
	defer func() { flag.CommandLine = previousFlags; os.Args = previousArgs }()
	flag.CommandLine = flag.NewFlagSet("admin-reset-test", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{"admin-reset-test", "-username", "t1_reset_admin", "-password-file", file, "-offline-confirmed"}
	if e = run(); e != nil {
		t.Fatal(e)
	}
	if !key.VerifySignature(token) {
		t.Fatal("fixture signature changed unexpectedly")
	}
	if _, e = repo.Session(ctx, digest[:]); !errors.Is(e, accounts.ErrCredentials) {
		t.Fatal("old session survived reset", e)
	}
	out, e := repo.TouchSession(ctx, digest[:], accounts.SessionMetadata{})
	if e != nil || out.Renewed {
		t.Fatal("reset session renewed", e)
	}
	var after time.Time
	if e = db.QueryRow(ctx, `SELECT expires_at FROM administrator_sessions WHERE token_digest=$1`, digest[:]).Scan(&after); e != nil || !before.Equal(after) {
		t.Fatal("reset session expiry changed", e)
	}
	if _, e = repo.Authenticate(ctx, "t1_reset_admin", "replacement-password"); e != nil {
		t.Fatal("reset password was not applied", e)
	}
}
