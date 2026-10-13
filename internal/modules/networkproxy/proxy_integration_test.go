package networkproxy

import (
	"context"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestEncryptedProxySettingsRevisionAndRekey(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB")
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
	v, _ := secrets.New(strings.Repeat("9", 64))
	repo := Repository{DB: db, Vault: v}
	var raw []byte
	var rev int64
	var tmdb, sites bool
	db.QueryRow(ctx, `SELECT url_ciphertext,revision,tmdb_enabled,sites_enabled FROM network_proxy_settings WHERE id='main'`).Scan(&raw, &rev, &tmdb, &sites)
	defer db.Exec(ctx, `UPDATE network_proxy_settings SET url_ciphertext=$1,revision=$2,tmdb_enabled=$3,sites_enabled=$4 WHERE id='main'`, raw, rev, tmdb, sites)
	out, err := repo.Save(ctx, Record{URL: "http://user:owned-secret@127.0.0.1:7890", TMDB: true, Revision: rev})
	if err != nil {
		t.Fatal(err)
	}
	var cipher []byte
	db.QueryRow(ctx, `SELECT url_ciphertext FROM network_proxy_settings WHERE id='main'`).Scan(&cipher)
	if strings.Contains(string(cipher), "owned-secret") {
		t.Fatal("plaintext at rest")
	}
	if _, err = repo.Save(ctx, Record{Revision: rev}); err != ErrRevision {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx)
	if err != nil || got.URL != out.URL || !got.TMDB || got.Sites {
		t.Fatal(got, err)
	}
	next, _ := secrets.New(strings.Repeat("8", 64))
	values, err := v.Open("network-proxy:main", cipher)
	if err != nil {
		t.Fatal(err)
	}
	newCipher, _ := next.Seal("network-proxy:main", values)
	if _, err = next.Open("network-proxy:main", newCipher); err != nil {
		t.Fatal(err)
	}
}
