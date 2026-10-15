package integrations

import (
	"context"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestTMDBSingleTokenEvenForDisabledAndConcurrentCreates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	if _, err = db.Exec(ctx, `DELETE FROM integrations WHERE kind='tmdb'`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE kind='tmdb'`)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := Repository{DB: db, Vault: vault}
	input := Input{Kind: "tmdb", Name: "Owned singleton test", Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "owned-token"}}
	var mu sync.Mutex
	success := []Record{}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, e := repo.Save(ctx, "", input)
			if e == nil {
				mu.Lock()
				success = append(success, record)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(success) != 1 {
		t.Fatal("multiple TMDB records accepted", len(success))
	}
	current := success[0]
	if _, err = repo.Save(ctx, "", input); err == nil {
		t.Fatal("disabled duplicate accepted")
	}
	input.Revision = current.Revision
	input.Secrets = map[string]string{"token": "updated-owned-token"}
	if _, err = repo.Save(ctx, current.ID, input); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Get(ctx, current.ID)
	if err != nil || updated.Values["token"] != "updated-owned-token" {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM integrations WHERE kind='tmdb'`).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
}
