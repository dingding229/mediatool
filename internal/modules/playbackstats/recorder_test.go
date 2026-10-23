package playbackstats

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"os"
	"strings"
	"testing"
	"time"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil || c.ConnConfig.Database != "media_tool_test" {
		t.Fatal("test database only")
	}
	db, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	return db
}
func TestMergeDurationDedupStatisticsDeletionAndRetention(t *testing.T) {
	db := testPool(t)
	ctx := t.Context()
	ids := []string{jobs.UUID(), jobs.UUID()}
	devices := []string{jobs.UUID(), jobs.UUID()}
	for i, id := range ids {
		_, e := db.Exec(ctx, `INSERT INTO media_users(id,username,display_name,password_hash,must_change_password)VALUES($1,$2,'F7 synthetic','not-a-real-hash',false)`, id, "f7-test-"+id)
		if e != nil {
			t.Fatal(e)
		}
		_, e = db.Exec(ctx, `INSERT INTO media_user_devices(id,user_id,device_key,client)VALUES($1,$2,'f7-device','SenPlayer')`, devices[i], id)
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Cleanup(func() {
		_, e := db.Exec(context.Background(), `DELETE FROM media_users WHERE id=ANY($1::uuid[])`, ids)
		if e != nil {
			t.Error(e)
		}
	})
	r := New(db, slog.Default())
	at := time.Now().Add(-2 * time.Minute)
	s, e := 0, 4
	v := Snapshot{ID: jobs.UUID(), UserID: ids[0], DeviceID: devices[0], ItemID: "123456", SeriesID: "12", SeasonID: "13", Type: "episode", Name: "合成剧", EpisodeTitle: "合成集名", Season: &s, Episode: &e, IP: "8.8.8.8", State: "playing", At: at, Redirect: true}
	write := func(v Snapshot) {
		t.Helper()
		if e := r.Write(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	write(Snapshot{DeviceOnly: true, UserID: ids[0], DeviceID: devices[0], IP: "8.8.8.8", At: at})
	write(Snapshot{DeviceOnly: true, UserID: ids[0], DeviceID: devices[0], IP: "8.8.8.8", At: at.Add(time.Second)})
	write(v)
	write(v)
	v.Redirect = false
	v.At = at.Add(20 * time.Second)
	v.Position = 20 * 10000000
	write(v)
	write(v)
	v.At = at.Add(30 * time.Second)
	v.State = "paused"
	v.Position = 30 * 10000000
	write(v)
	v.At = at.Add(90 * time.Second)
	v.State = "playing"
	write(v)
	v.At = at.Add(100 * time.Second)
	v.State = "stopped"
	v.Position = 40 * 10000000
	v.IP = "1.1.1.1"
	write(v)
	write(v)
	f := Filter{UserID: ids[0], Days: 7, Limit: 25}
	repo := Repository{DB: db}
	summary, err := repo.Summary(ctx, f)
	if err != nil || summary.Count != 1 || summary.Seconds != 40 || summary.Series != 1 || summary.Episodes != 1 || summary.Movies != 0 {
		t.Fatal(summary, err)
	}
	page, err := repo.Events(ctx, f)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].RedirectCount != 2 || page.Items[0].Client != "SenPlayer" || page.Items[0].Season == nil || *page.Items[0].Season != 0 {
		t.Fatal(page, err)
	}
	ips, err := repo.IPs(ctx, f)
	if err != nil || ips.Total != 2 || ips.Items[0].Count != 1 {
		t.Fatal(ips, err)
	}
	grouped, err := repo.GroupedIPs(ctx, f, func(string) string { return "合成国家" })
	if err != nil || grouped.Total != 1 || grouped.Items[0].Count != 1 || grouped.Items[0].Devices != 1 {
		t.Fatal(grouped, err)
	}
	f.Start = 25
	page, err = repo.Events(ctx, f)
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatal("page total", page, err)
	}
	f.Start = 0
	deviceIPs, err := repo.DeviceIPs(ctx, f)
	if err != nil || deviceIPs.Total != 1 || deviceIPs.Items[0].Count != 2 {
		t.Fatal("device request history", deviceIPs, err)
	}
	// Retained user statistics survive disable and grant revocation.
	if _, err = db.Exec(ctx, `UPDATE media_users SET enabled=false WHERE id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	f.Start = 0
	summary, err = repo.Summary(ctx, f)
	if err != nil || summary.Count != 1 {
		t.Fatal("disable erased history", err)
	}
	second := v
	second.ID = jobs.UUID()
	second.UserID = ids[1]
	second.DeviceID = devices[1]
	second.Type = "movie"
	second.SeriesID = ""
	second.Season = nil
	second.Episode = nil
	second.ItemID = "999999"
	second.IP = "10.1.2.3"
	write(second)
	page, err = repo.Events(ctx, Filter{UserID: ids[1], Limit: 25})
	if err != nil || page.Items[0].IP != "" {
		t.Fatal("private IP retained", page, err)
	}
	// User deletion is the real repository operation; the cascade commits atomically.
	if err = (mediausers.Repository{DB: db}).Mutate(ctx, ids[0], 1, "delete", ""); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM media_playback_ips WHERE event_id=$1`, v.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("IP cascade", n, err)
	}
	deviceIPs, err = repo.DeviceIPs(ctx, f)
	if err != nil || deviceIPs.Total != 0 {
		t.Fatal("device IP cascade", deviceIPs, err)
	}
	summary, err = repo.Summary(ctx, f)
	if err != nil || summary.Count != 0 {
		t.Fatal("events cascade", summary, err)
	}
	summary, err = repo.Summary(ctx, Filter{UserID: ids[1], Limit: 25})
	if err != nil || summary.Count != 1 {
		t.Fatal("other user changed", err)
	}
	// Stale convergence and batch pruning are bounded and safe on repeated runs.
	if _, err = db.Exec(ctx, `UPDATE media_playback_events SET state='playing',ended_at=NULL,last_seen_at=now()-interval '6 minutes' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if err = r.Maintain(ctx, time.Now(), 180); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM media_playback_events WHERE id=$1`, second.ID).Scan(&state); err != nil || state != "stopped" {
		t.Fatal("stale convergence", err)
	}
	if _, err = db.Exec(ctx, `UPDATE media_playback_events SET started_at=now()-interval '181 days' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if err = r.Maintain(ctx, time.Now(), 180); err != nil {
		t.Fatal(err)
	}
	summary, err = repo.Summary(ctx, Filter{UserID: ids[1], Limit: 25})
	if err != nil || summary.Count != 0 {
		t.Fatal("retention", summary, err)
	}
}
func TestRecorderFailureIsNonblockingAndLogsNoSecrets(t *testing.T) {
	db := testPool(t)
	c := db.Config()
	broken, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	broken.Close()
	var logs bytes.Buffer
	r := New(broken, slog.New(slog.NewJSONHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx, func(context.Context) (int, error) { return 0, errors.New("fail") })
	}()
	start := time.Now()
	for i := 0; i < 1100; i++ {
		r.Submit(Snapshot{ID: "PRIVATE_CANARY"})
	}
	if time.Since(start) > time.Second {
		t.Fatal("statistics backpressured playback")
	}
	cancel()
	<-done
	if strings.Contains(logs.String(), "PRIVATE_CANARY") {
		t.Fatal("sensitive payload logged")
	}
}
