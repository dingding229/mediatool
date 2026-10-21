package externalapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if listErr != nil || len(listed) != 1 || listed[0].ExpiresAt != nil || listed[0].Prefix != k.Token[:15] {
		t.Fatal("permanent key readback", listErr)
	}
	assertSafeList := func() {
		t.Helper()
		// Listing must not need the encryption key, even for old/revoked credentials.
		listed, err := (Repository{DB: db}).List(ctx)
		if err != nil || len(listed) != 1 {
			t.Fatal("metadata-only list", err)
		}
		wire, err := json.Marshal(listed)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(k.Token))
		for _, forbidden := range []string{k.Token, hex.EncodeToString(sum[:]), `"token"`, `"tokenDigest"`, `"token_digest"`, `"token_ciphertext"`} {
			if strings.Contains(string(wire), forbidden) {
				t.Fatalf("credential material leaked in list: field/value %q", forbidden)
			}
		}
		if listed[0].Prefix != k.Prefix || listed[0].Name != k.Name || len(listed[0].Scopes) != 1 {
			t.Fatal("list metadata changed")
		}
	}
	created, err := json.Marshal(k)
	if err != nil || !strings.Contains(string(created), `"token":"`+k.Token+`"`) {
		t.Fatal("creation lost one-time plaintext", err)
	}
	assertSafeList()
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
	assertSafeList()
	if _, _, err = repo.Admit(ctx, k.Token, "status.read", "GET", "/status"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, _, err = repo.Admit(ctx, "wrong", "status.read", "GET", "/status"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}
