package networkproxy

import (
	"context"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"sync"
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
	out, err := repo.Save(ctx, Update{Fields: Fields{Protocol: "http", Host: "127.0.0.1", Port: 7890, Username: "user"}, Password: "owned-secret", TMDB: true, Revision: rev})
	if err != nil {
		t.Fatal(err)
	}
	var cipher []byte
	db.QueryRow(ctx, `SELECT url_ciphertext FROM network_proxy_settings WHERE id='main'`).Scan(&cipher)
	if strings.Contains(string(cipher), "owned-secret") {
		t.Fatal("plaintext at rest")
	}
	if _, err = repo.Save(ctx, Update{Revision: rev}); err != ErrRevision {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx)
	if err != nil || got.proxyURL() != out.proxyURL() || !got.TMDB || got.Sites {
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

func TestI17LegacyCiphertextAndPasswordRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	if e = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); e != nil || name != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	var original []byte
	var revision int64
	var tmdb, sites bool
	if e = db.QueryRow(ctx, `SELECT url_ciphertext,revision,tmdb_enabled,sites_enabled FROM network_proxy_settings`).Scan(&original, &revision, &tmdb, &sites); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, `UPDATE network_proxy_settings SET url_ciphertext=$1,revision=$2,tmdb_enabled=$3,sites_enabled=$4`, original, revision, tmdb, sites)
	v, _ := secrets.New(strings.Repeat("9", 64))
	repo := Repository{DB: db, Vault: v}
	for _, legacy := range []string{"http://u:p@proxy:80", "https://proxy:443", "socks5://proxy:1080", "socks5h://proxy:1080"} {
		raw, _ := v.Seal("network-proxy:main", map[string]string{"url": legacy})
		if _, e = db.Exec(ctx, `UPDATE network_proxy_settings SET url_ciphertext=$1,tmdb_enabled=true`, raw); e != nil {
			t.Fatal(e)
		}
		old, e := repo.Get(ctx)
		if e != nil || !old.NeedsInput || !old.TMDB {
			t.Fatal("legacy flags")
		}
		if _, e = Client(old, "tmdb"); e != ErrNeedsInput {
			t.Fatal("not fail closed")
		}
		toggled, e := repo.Save(ctx, Update{Sites: true, Revision: old.Revision})
		if e != nil || !toggled.NeedsInput || !toggled.Sites || toggled.TMDB {
			t.Fatal("route update")
		}
		var retained []byte
		db.QueryRow(ctx, `SELECT url_ciphertext FROM network_proxy_settings`).Scan(&retained)
		if string(raw) != string(retained) {
			t.Fatal("legacy ciphertext not retained")
		}
		replacement := Update{Fields: Fields{Protocol: "socks5", Host: "127.0.0.1", Port: 1080, Username: "u@/"}, Password: "secret@/:?#", Revision: toggled.Revision}
		saved, e := repo.Save(ctx, replacement)
		if e != nil || saved.NeedsInput || !saved.HasPassword {
			t.Fatal(e)
		}
		replacement.Revision = saved.Revision
		replacement.Password = ""
		kept, e := repo.Save(ctx, replacement)
		if e != nil || kept.Password != saved.Password {
			t.Fatal("password preservation")
		}
		replacement.Revision = kept.Revision
		replacement.ClearPassword = true
		cleared, e := repo.Save(ctx, replacement)
		if e != nil || cleared.HasPassword || cleared.Password != "" {
			t.Fatal("password clear")
		}
	}
	current, e := repo.Get(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, password := range []string{"concurrent-one", "concurrent-two"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, err := repo.Save(ctx, Update{Fields: current.Fields, Password: value, Revision: current.Revision})
			results <- err
		}(password)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if err == ErrRevision {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("concurrent revision guard", success, conflicts)
	}

}
