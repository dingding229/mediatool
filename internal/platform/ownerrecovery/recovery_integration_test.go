package ownerrecovery

import (
	"bytes"
	"context"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/database"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPostgresEmptyMarkerSingleUseAndExpiry(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("test DB required")
	}
	db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE")
	defer db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE")
	repo := accounts.Repository{DB: db}
	a, err := repo.Register(ctx, "owned_admin", "Original-2026", "owned-session")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "password")
	var console bytes.Buffer
	if applied, err := Apply(ctx, db, marker, &console); err != nil || applied {
		t.Fatal("absent marker")
	}
	os.WriteFile(marker, []byte("not-empty"), 0600)
	if _, err = Apply(ctx, db, marker, &console); err == nil {
		t.Fatal("nonempty reset")
	}
	os.Remove(marker)
	os.Symlink(filepath.Join(dir, "other"), marker)
	if _, err = Apply(ctx, db, marker, &console); err == nil {
		t.Fatal("symlink reset")
	}
	os.Remove(marker)
	os.WriteFile(marker, nil, 0600)
	applied, err := Apply(ctx, db, marker, &console)
	if err != nil || !applied {
		t.Fatal(applied, err)
	}
	match := regexp.MustCompile(`Temporary password: ([A-Za-z0-9_-]{20})`).FindStringSubmatch(console.String())
	if len(match) != 2 {
		t.Fatal("password not shown once")
	}
	temporary := match[1]
	if _, err = os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("marker retained")
	}
	next, err := repo.Authenticate(ctx, a.Username, temporary)
	if err != nil || !next.MustChangePassword {
		t.Fatal("temporary password refused", err)
	}
	if _, err = repo.Authenticate(ctx, a.Username, "Original-2026"); err == nil {
		t.Fatal("old password active")
	}
	console.Reset()
	if applied, err = Apply(ctx, db, marker, &console); err != nil || applied || console.Len() > 0 {
		t.Fatal("marker replay")
	}
	if err = repo.ChangePassword(ctx, next, temporary, "New-owned-2026"); err != nil {
		t.Fatal(err)
	}
	next, err = repo.Authenticate(ctx, a.Username, "New-owned-2026")
	if err != nil || next.MustChangePassword {
		t.Fatal(next, err)
	}
	os.WriteFile(marker, nil, 0600)
	if _, err = Apply(ctx, db, marker, &console); err != nil {
		t.Fatal(err)
	}
	second := regexp.MustCompile(`Temporary password: ([A-Za-z0-9_-]{20})`).FindStringSubmatch(console.String())[1]
	db.Exec(ctx, `UPDATE administrator_accounts SET temporary_password_expires_at=now()-interval '1 second'`)
	if _, err = repo.Authenticate(ctx, a.Username, second); !errors.Is(err, accounts.ErrCredentials) {
		t.Fatal("expired temporary accepted")
	}
	var audit string
	db.QueryRow(ctx, `SELECT coalesce(string_agg(details::text,' '),'') FROM audit_logs WHERE action='account.owner_recovery'`).Scan(&audit)
	if strings.Contains(audit, temporary) || strings.Contains(audit, second) {
		t.Fatal("audit leak")
	}
}
