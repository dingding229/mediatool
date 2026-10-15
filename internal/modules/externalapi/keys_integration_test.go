package externalapi

import (
	"context"
	"errors"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestPermanentScopedAPIKeysRevokeAndLimit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database")
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
		t.Fatal(name)
	}
	db.Exec(ctx, "TRUNCATE external_api_keys CASCADE")
	defer db.Exec(ctx, "TRUNCATE external_api_keys CASCADE")
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := Repository{DB: db, Vault: vault}
	k, err := repo.Create(ctx, "Owned API", []string{"status.read"})
	if err != nil {
		t.Fatal(err)
	}
	if k.ExpiresAt != nil {
		t.Fatal("permanent key has expiration")
	}
	listed, listErr := repo.List(ctx)
	if listErr != nil || len(listed) != 1 || listed[0].ExpiresAt != nil || listed[0].Token != k.Token {
		t.Fatal("permanent key readback", listErr)
	}
	if _, _, err = repo.Admit(ctx, k.Token, "downloads.run", "POST", "/download"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, _, err = repo.Admit(ctx, k.Token, "status.read", "GET", "/status"); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 119; n++ {
		if _, _, err = repo.Admit(ctx, k.Token, "status.read", "GET", "/status"); err != nil {
			t.Fatal(n, err)
		}
	}
	if _, _, err = repo.Admit(ctx, k.Token, "status.read", "GET", "/status"); !errors.Is(err, ErrRate) {
		t.Fatal(err)
	}
	if err = repo.Revoke(ctx, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.Admit(ctx, k.Token, "status.read", "GET", "/status"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, _, err = repo.Admit(ctx, "wrong", "status.read", "GET", "/status"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}
