package messaging

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/database"
)

func TestNoticePageProjectsWithoutChangingStoredOrDeliveredBody(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	if e = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); e != nil || name != "media_tool_test" {
		t.Fatal("isolated database required", e)
	}
	event, id := jobs.UUID(), jobs.UUID()
	raw := "message: 宿主内存使用率过高；指标 memoryPercent，阈值 90.00\nassetId: " + jobs.UUID()
	if _, e = db.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'ops.alert.active',$1,'{}')`, event); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, `DELETE FROM outbox_events WHERE id=$1`, event)
	defer db.Exec(ctx, `DELETE FROM notification_messages WHERE id=$1`, id)
	if _, e = db.Exec(ctx, `INSERT INTO notification_messages(id,event_id,title,body)VALUES($1,$2,'系统运维告警',$3)`, id, event, raw); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	ro, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	rows, e := (Dispatcher{DB: ro}).Page(ctx, 0, 200)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, row := range rows {
		if row.ID == id {
			found = true
			if row.Body != "内存使用率触发告警（阈值 90%）" || strings.Contains(row.Body, "assetId") {
				t.Fatal(row)
			}
		}
	}
	if !found {
		t.Fatal("projected notice missing")
	}
	var after string
	if e = db.QueryRow(ctx, `SELECT body FROM notification_messages WHERE id=$1`, id).Scan(&after); e != nil || after != raw {
		t.Fatal("stored delivery body changed", e)
	}
}
