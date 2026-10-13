package settings

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestPostgresGlobalSubscriptionCronAtomicAndPreservesIdentity(t *testing.T) {
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
	var n string
	db.QueryRow(ctx, "select current_database()").Scan(&n)
	if n != "media_tool_test" {
		t.Fatal(n)
	}
	repo := Repository{DB: db}
	before, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `DELETE FROM subscriptions WHERE title='Owned interval fixture'`)
	defer db.Exec(ctx, `DELETE FROM subscriptions WHERE title='Owned interval fixture'`)
	defer func() { latest, _ := repo.Get(ctx); repo.Save(ctx, before.Value, latest.Revision) }()
	_, err = db.Exec(ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled,source,interval_seconds)VALUES(gen_random_uuid(),'Owned interval fixture','movie','{"id":1,"title":"Owned Film"}','{"accountIds":[],"include":["Owned"],"freeOnly":true}',false,'search',1800)`)
	if err != nil {
		t.Fatal(err)
	}
	value := before.Value
	value.SubscriptionCron = "0 9,21 * * *"
	record, err := repo.Save(ctx, value, before.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var interval int
	var enabled bool
	var rules, identity string
	db.QueryRow(ctx, `SELECT interval_seconds,enabled,rules::text,identity::text FROM subscriptions WHERE title='Owned interval fixture'`).Scan(&interval, &enabled, &rules, &identity)
	if interval != 1800 || enabled || rules == "{}" || identity == "{}" {
		t.Fatal(interval, enabled, rules, identity)
	}
	if _, err = repo.Save(ctx, before.Value, before.Revision); err != ErrRevision {
		t.Fatal("lost update accepted", err)
	}
	if record.Value.SubscriptionCron != "0 9,21 * * *" {
		t.Fatal(record)
	}
}
