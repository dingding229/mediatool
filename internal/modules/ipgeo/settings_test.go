package ipgeo

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestEncryptedKeyPreservationAndRekey(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil || c.ConnConfig.Database != "media_tool_test" {
		t.Fatal("test database only")
	}
	schema := "f7_geo_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	base, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(base.Close)
	if _, e = base.Exec(t.Context(), `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := base.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); e != nil {
			t.Error(e)
		}
	})
	c.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	if _, e = db.Exec(t.Context(), `CREATE TABLE ip_geo_settings(id text PRIMARY KEY DEFAULT 'cz88-community',singleton boolean UNIQUE DEFAULT true, encrypted_credentials bytea,updated_at timestamptz DEFAULT now())`); e != nil {
		t.Fatal(e)
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	s := New(db, vault, t.TempDir())
	t.Cleanup(func() {
		if _, e := db.Exec(context.Background(), `DELETE FROM ip_geo_settings WHERE id='cz88-community'`); e != nil {
			t.Error(e)
		}
	})
	if e = s.Save(t.Context(), Credentials{"PRIVATE_DOWNLOAD_KEY", fixtureKey}); e != nil {
		t.Fatal(e)
	}
	var raw []byte
	if e = db.QueryRow(t.Context(), `SELECT encrypted_credentials FROM ip_geo_settings`).Scan(&raw); e != nil || strings.Contains(string(raw), "PRIVATE_DOWNLOAD_KEY") {
		t.Fatal("encryption failed", e)
	}
	if e = s.Save(t.Context(), Credentials{DownloadKey: "NEW_PRIVATE_KEY"}); e != nil {
		t.Fatal(e)
	}
	got, e := s.credentials(t.Context())
	if e != nil || got.DecryptKey != fixtureKey || got.DownloadKey != "NEW_PRIVATE_KEY" {
		t.Fatal("blank key replacement", e)
	}
	// Cipher catalog enrollment is what restore validation and offline rotation use.
	found := false
	for _, v := range secrets.CipherColumns {
		if v.Table == "ip_geo_settings" && v.Column == "encrypted_credentials" {
			found = true
		}
	}
	if !found {
		t.Fatal("key omitted from restore/rekey validation")
	}
}
