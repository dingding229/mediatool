package playbackstats

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func TestIPReportMergesVerified302ReadOnlyAndRespectsIdentityAndThirtyDays(t *testing.T) {
	db := testPool(t)
	ctx := t.Context()
	user, device := jobs.UUID(), jobs.UUID()
	if _, e := db.Exec(ctx, `INSERT INTO media_users(id,username,display_name,password_hash,must_change_password)VALUES($1,$2,'I42 user','synthetic',false)`, user, "i42-"+user); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, `INSERT INTO media_user_devices(id,user_id,device_key,name,client)VALUES($1,$2,'i42','i42 device','test')`, device, user); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Exec(context.Background(), `DELETE FROM media_users WHERE id=$1`, user) })
	now := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	recorder := New(db, slog.Default())
	for _, at := range []time.Time{now.Add(-40 * 24 * time.Hour), now.Add(-time.Hour), now.Add(-2 * time.Hour)} {
		if e := recorder.Write(ctx, Snapshot{ID: jobs.UUID(), UserID: user, DeviceID: device, ItemID: "123456", Type: "movie", Name: "I42", State: "playing", IP: "8.8.8.8", At: at}); e != nil {
			t.Fatal(e)
		}
	}
	ids := []string{}
	insert := func(ip, source string, public, allowed bool, status int, at time.Time) {
		t.Helper()
		id := jobs.UUID()
		ids = append(ids, id)
		_, e := db.Exec(ctx, `INSERT INTO player_access_events(id,observation_id,user_agent,client_ip,entry,method,allowed,reason,policy_revision,status,created_at,connection_ip,ip_source,ip_public) VALUES($1,repeat('4',64),'I42',$2,'strm','GET',$3,'test',1,$4,$5,'10.0.0.2',$6,$7)`, id, ip, allowed, status, at, source, public)
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Cleanup(func() {
		db.Exec(context.Background(), `DELETE FROM player_access_events WHERE id=ANY($1::uuid[])`, ids)
	})
	insert("8.8.8.8", "direct", true, true, 302, now.AddDate(0, 0, -30)) // Inclusive boundary.
	insert("8.8.8.8", "trusted_proxy", true, true, 302, now.Add(-time.Minute))
	insert("223.5.5.5", "trusted_proxy", true, true, 302, now.Add(-time.Minute))
	insert("223.6.6.6", "direct", true, true, 302, now.AddDate(0, 0, -30).Add(-time.Microsecond))
	insert("8.8.8.8", "direct", true, true, 302, now.Add(time.Second))
	insert("9.9.9.9", "direct", true, false, 302, now)
	insert("1.1.1.1", "legacy_unknown", true, true, 302, now)
	insert("8.8.4.4", "direct", false, true, 302, now)
	insert("10.0.0.1", "direct", true, true, 302, now) // Even corrupt public=true cannot include LAN.
	insert("1.0.0.1", "direct", true, true, 200, now)
	cfg := db.Config().Copy()
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	ro, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	repository := Repository{DB: ro}
	f := Filter{Days: 30, Limit: 25}
	before := 0
	db.QueryRow(ctx, `SELECT count(*)FROM player_access_events WHERE id=ANY($1::uuid[])`, ids).Scan(&before)
	out, e := repository.ipReportAt(ctx, f, reportLocation, now)
	if e != nil {
		t.Fatal(e)
	}
	if out.Total != 2 || out.Summary.UniqueIPs != 2 || out.Summary.ActiveUsers != 1 {
		t.Fatalf("%+v", out)
	}
	for _, row := range out.Items {
		switch row.IP {
		case "8.8.8.8":
			if row.SessionCount != 2 || row.Direct302Count != 2 || row.Count != 4 || row.Users != 1 || row.NewIP {
				t.Fatalf("%+v", row)
			}
		case "223.5.5.5":
			if row.SessionCount != 0 || row.Direct302Count != 1 || row.Users != 0 || len(row.Accounts) != 0 {
				t.Fatalf("%+v", row)
			}
		default:
			t.Fatal("unverified/private/denied/out-of-window IP", row.IP)
		}
	}
	for _, entry := range out.Summary.Provinces {
		if entry.Count != 1 {
			t.Fatal("ranking must deduplicate IPs", entry)
		}
	}
	for _, filter := range []Filter{{UserID: user, Days: 30, Limit: 25}, {DeviceID: device, Days: 30, Limit: 25}} {
		result, e := repository.ipReportAt(ctx, filter, reportLocation, now)
		if e != nil || len(result.Items) != 1 || result.Items[0].Direct302Count != 0 || result.Items[0].SessionCount != 2 {
			t.Fatal(result, e)
		}
	}
	after := 0
	db.QueryRow(ctx, `SELECT count(*)FROM player_access_events WHERE id=ANY($1::uuid[])`, ids).Scan(&after)
	if before != after {
		t.Fatal("read-only sources were modified")
	}
}
