package backupverify

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
)

func TestPostgresAuthorizedSubscriptionProjectionStillDetectsIdentityAndSafetyRuleChanges(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL")
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
	var id string
	err = db.QueryRow(ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled,source,interval_seconds)VALUES(gen_random_uuid(),'Owned migration fixture','movie','{"id":37,"title":"Owned Film"}','{"minSize":10,"maxSize":100,"minSeeders":5,"freeOnly":true,"allowUnknownHR":false}',false,'search',300) RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, id)
	baseline, err := Inventory(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := subscriptionMigrationProjection(ctx, db, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE subscriptions SET rules=rules||'{"minSize":0,"maxSize":0,"minSeeders":0}',interval_seconds=1800,revision=revision+1,updated_at=now()WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	after, err := subscriptionMigrationProjection(ctx, db, baseline)
	if err != nil || !equal(before, after, false) {
		t.Fatal("authorized fields not isolated", err)
	}
	db.Exec(ctx, `UPDATE subscriptions SET rules=jsonb_set(rules,'{freeOnly}','false') WHERE id=$1`, id)
	corrupt, err := subscriptionMigrationProjection(ctx, db, baseline)
	if err != nil || equal(before, corrupt, false) {
		t.Fatal("free/H&R guard bypassed", err)
	}
}
