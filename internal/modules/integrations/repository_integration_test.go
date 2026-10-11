package integrations

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestPostgresEncryptedConnectionLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for real PostgreSQL test")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := Repository{DB: db, Vault: vault}
	in := Input{Kind: "qbittorrent", Name: "repository test", Settings: map[string]any{"endpoint": "http://127.0.0.1:9999"}, Secrets: map[string]string{"password": "credential-secret"}}
	record, err := repo.Save(ctx, "", in)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, record.ID)
	raw, _ := json.Marshal(record)
	if strings.Contains(string(raw), "credential-secret") {
		t.Fatal("public DTO leaked credentials")
	}
	var encrypted []byte
	if err = db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, record.ID).Scan(&encrypted); err != nil || strings.Contains(string(encrypted), "credential-secret") {
		t.Fatal("database plaintext credential")
	}
	fetched, err := repo.Get(ctx, record.ID)
	if err != nil || fetched.Values["password"] != "credential-secret" {
		t.Fatal(err)
	}
	in.Revision = record.Revision
	in.Name = "updated"
	in.Secrets = nil
	updated, err := repo.Save(ctx, record.ID, in)
	if err != nil || updated.Revision != record.Revision+1 {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, record.ID, in); err != ErrRevision {
		t.Fatal("stale update allowed", err)
	}
	in.Revision = updated.Revision
	in.Settings["endpoint"] = "http://127.0.0.1:8888"
	if _, err = repo.Save(ctx, record.ID, in); err == nil {
		t.Fatal("retained secrets when endpoint changed")
	}
	if err = repo.Delete(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, record.ID); err != ErrNotFound {
		t.Fatal(err)
	}
}
